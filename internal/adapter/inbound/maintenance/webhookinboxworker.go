package maintenance

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	gh "github.com/google/go-github/v90/github"
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/inbound/webhook"
	"github.com/it-play/sandrone-code-review-bot/internal/core/backoff"
	"github.com/it-play/sandrone-code-review-bot/internal/core/port/outbound"
	"github.com/it-play/sandrone-code-review-bot/internal/core/webhookinbox"
)

const webhookInboxPollInterval = time.Second
const webhookInboxLease = 5 * time.Minute
const webhookInboxProcessTimeout = 90 * time.Second
const webhookInboxFinalizeTimeout = 5 * time.Second
const webhookInboxCleanupInterval = time.Hour
const webhookInboxRejectRetention = 30 * 24 * time.Hour
const webhookInboxOrphanAfter = 7 * 24 * time.Hour
const webhookInboxConcurrency = 2
const webhookInboxCleanupBatch = 100

var webhookInboxBackoff = backoff.Policy{Initial: 5 * time.Second, Maximum: time.Hour}

type WebhookInboxWorker struct {
	repository outbound.WebhookInboxRepository
	router     *webhook.EventRouter
	clock      outbound.Clock
	masker     outbound.Masker
	metrics    WebhookMetrics
	logger     *slog.Logger
	retention  time.Duration
}

func NewWebhookInboxWorker(repository outbound.WebhookInboxRepository, router *webhook.EventRouter, clock outbound.Clock, masker outbound.Masker, metrics WebhookMetrics, logger *slog.Logger, retention time.Duration) *WebhookInboxWorker {
	return &WebhookInboxWorker{
		repository: repository,
		router:     router,
		clock:      clock,
		masker:     masker,
		metrics:    metrics,
		logger:     logger,
		retention:  retention,
	}
}

func (w *WebhookInboxWorker) Run(ctx context.Context) {
	var workers sync.WaitGroup
	for range webhookInboxConcurrency {
		workers.Add(1)
		go func() {
			defer workers.Done()
			w.consume(ctx)
		}()
	}
	w.cleanup(ctx)
	ticker := time.NewTicker(webhookInboxCleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			workers.Wait()
			return
		case <-ticker.C:
			w.cleanup(ctx)
		}
	}
}

func (w *WebhookInboxWorker) consume(ctx context.Context) {
	ticker := time.NewTicker(webhookInboxPollInterval)
	defer ticker.Stop()
	for {
		processed := w.claimAndProcess(ctx)
		if processed {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *WebhookInboxWorker) claimAndProcess(ctx context.Context) bool {
	now := w.clock.Now()
	delivery, found, err := w.repository.ClaimNext(ctx, now, now.Add(webhookInboxLease))
	if err != nil {
		if ctx.Err() == nil {
			w.logger.Error("웹훅 inbox delivery를 선점하지 못했습니다", "error", err)
		}
		return false
	}
	if !found {
		return false
	}
	w.process(ctx, delivery)
	return true
}

func (w *WebhookInboxWorker) process(ctx context.Context, delivery webhookinbox.Delivery) {
	if !delivery.ReceivedAt.After(w.clock.Now().Add(-webhookInboxOrphanAfter)) {
		w.reject(ctx, delivery, fmt.Errorf("7일의 웹훅 delivery 처리 기한이 지났습니다"))
		return
	}
	processContext, cancel := context.WithTimeout(ctx, webhookInboxProcessTimeout)
	event, parseErr := gh.ParseWebHook(delivery.EventType, delivery.Payload)
	if parseErr != nil {
		cancel()
		w.reject(ctx, delivery, parseErr)
		return
	}
	action, routeErr := w.router.Route(processContext, event, delivery.RequestIdentity, delivery.ReceivedAt)
	cancel()
	if routeErr != nil {
		w.metrics.ObserveWebhook(delivery.EventType, action+"_retry")
		w.retry(ctx, delivery, routeErr)
		return
	}
	w.metrics.ObserveWebhook(delivery.EventType, action)
	w.complete(ctx, delivery)
}

func (w *WebhookInboxWorker) complete(ctx context.Context, delivery webhookinbox.Delivery) {
	now := w.clock.Now()
	finalizeContext, cancel := webhookInboxFinalizeContext(ctx)
	defer cancel()
	if err := w.repository.Complete(finalizeContext, delivery.ID, delivery.LeaseToken, now, now.Add(w.retention)); err != nil {
		w.logger.Error("웹훅 inbox delivery를 완료하지 못했습니다", "delivery", delivery.Key, "error", err)
	}
}

func (w *WebhookInboxWorker) retry(ctx context.Context, delivery webhookinbox.Delivery, cause error) {
	now := w.clock.Now()
	if !delivery.ReceivedAt.After(now.Add(-webhookInboxOrphanAfter)) {
		w.reject(ctx, delivery, fmt.Errorf("7일 동안 웹훅 delivery를 처리하지 못했습니다: %w", cause))
		return
	}
	availableAt := now.Add(webhookInboxRetryDelay(delivery.ID, delivery.Attempts))
	finalizeContext, cancel := webhookInboxFinalizeContext(ctx)
	defer cancel()
	if err := w.repository.Retry(finalizeContext, delivery.ID, delivery.LeaseToken, availableAt, w.masker.Mask(cause.Error())); err != nil {
		w.logger.Error("웹훅 inbox delivery 재시도를 저장하지 못했습니다", "delivery", delivery.Key, "error", err)
		return
	}
	w.logger.Warn("웹훅 inbox delivery 처리를 재시도합니다", "delivery", delivery.Key, "attempt", delivery.Attempts, "available_at", availableAt, "error", cause)
}

func (w *WebhookInboxWorker) reject(ctx context.Context, delivery webhookinbox.Delivery, cause error) {
	now := w.clock.Now()
	finalizeContext, cancel := webhookInboxFinalizeContext(ctx)
	defer cancel()
	if err := w.repository.Reject(finalizeContext, delivery.ID, delivery.LeaseToken, now, now.Add(webhookInboxRejectRetention), w.masker.Mask(cause.Error())); err != nil {
		w.logger.Error("처리할 수 없는 웹훅 inbox delivery를 격리하지 못했습니다", "delivery", delivery.Key, "error", err)
		return
	}
	w.metrics.ObserveWebhook(delivery.EventType, "rejected")
	w.logger.Error("처리할 수 없는 웹훅 inbox delivery를 격리했습니다", "delivery", delivery.Key, "error", cause)
}

func (w *WebhookInboxWorker) cleanup(ctx context.Context) {
	for {
		deleted, err := w.repository.DeleteExpired(ctx, w.clock.Now(), webhookInboxCleanupBatch)
		if err != nil {
			if ctx.Err() == nil {
				w.logger.Error("만료된 웹훅 inbox delivery를 제거하지 못했습니다", "error", err)
			}
			return
		}
		if deleted > 0 {
			w.logger.Info("만료된 웹훅 inbox delivery를 제거했습니다", "deliveries", deleted)
		}
		if deleted < webhookInboxCleanupBatch {
			return
		}
	}
}

func webhookInboxFinalizeContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), webhookInboxFinalizeTimeout)
}

func webhookInboxRetryDelay(deliveryID uint64, attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	return webhookInboxBackoff.Delay(attempt-1, deliveryID)
}
