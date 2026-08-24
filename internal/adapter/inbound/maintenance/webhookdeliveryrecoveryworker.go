package maintenance

import (
	"context"
	"log/slog"
	"sort"
	"strconv"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/port/outbound"
	"github.com/it-play/sandrone-code-review-bot/internal/core/webhookrecovery"
)

const webhookDeliveryRecoveryStartupDelay = 10 * time.Second
const webhookDeliveryRecoveryInterval = 5 * time.Minute
const webhookDeliveryRecoveryLookback = 30 * time.Minute
const webhookDeliveryRecoveryMinimumAge = 30 * time.Second
const webhookDeliveryRecoveryRepeatInterval = 10 * time.Minute
const webhookDeliveryRecoveryPassTimeout = 90 * time.Second
const webhookDeliveryRecoveryCallTimeout = 20 * time.Second
const webhookDeliveryRecoveryRateInterval = time.Second
const webhookDeliveryRecoveryScanPageLimit = 3
const webhookDeliveryRecoveryCandidateLimit = 100
const webhookDeliveryRecoveryRedeliveryLimit = 10

type WebhookDeliveryRecoveryWorker struct {
	recovery      outbound.WebhookDeliveryRecovery
	clock         outbound.Clock
	logger        *slog.Logger
	attempted     map[string]time.Time
	pending       map[string]webhookrecovery.Delivery
	scanCursor    string
	scanSucceeded map[string]time.Time
}

func NewWebhookDeliveryRecoveryWorker(recovery outbound.WebhookDeliveryRecovery, clock outbound.Clock, logger *slog.Logger) *WebhookDeliveryRecoveryWorker {
	return &WebhookDeliveryRecoveryWorker{
		recovery:      recovery,
		clock:         clock,
		logger:        logger,
		attempted:     map[string]time.Time{},
		pending:       map[string]webhookrecovery.Delivery{},
		scanSucceeded: map[string]time.Time{},
	}
}

func (w *WebhookDeliveryRecoveryWorker) Run(ctx context.Context) {
	startup := time.NewTimer(webhookDeliveryRecoveryStartupDelay)
	defer startup.Stop()
	select {
	case <-ctx.Done():
		return
	case <-startup.C:
		w.recover(ctx)
	}
	ticker := time.NewTicker(webhookDeliveryRecoveryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.recover(ctx)
		}
	}
}

func (w *WebhookDeliveryRecoveryWorker) recover(ctx context.Context) {
	now := w.clock.Now()
	w.forgetAttemptsBefore(now.Add(-webhookDeliveryRecoveryRepeatInterval))
	passContext, cancel := context.WithTimeout(ctx, webhookDeliveryRecoveryPassTimeout)
	defer cancel()
	currentAfter := now.Add(-webhookDeliveryRecoveryLookback)
	w.forgetPendingBefore(currentAfter)
	latest, err := w.recovery.Scan(passContext, "", webhookDeliveryRecoveryScanPageLimit)
	if err != nil {
		if ctx.Err() == nil {
			w.logger.Warn("GitHub App webhook delivery 복구 대상을 읽지 못했습니다", "error", err)
		}
		w.redeliverPending(passContext, ctx, now)
		return
	}
	latestDeliveries := webhookDeliveriesOnOrAfter(latest.Deliveries, currentAfter)
	latestSucceeded := webhookDeliverySuccesses(latestDeliveries)
	deliveries := append([]webhookrecovery.Delivery(nil), latestDeliveries...)
	succeeded := map[string]time.Time{}
	activeScan := w.scanCursor != ""
	if activeScan {
		for key, succeededAt := range w.scanSucceeded {
			if !succeededAt.Before(currentAfter) {
				succeeded[key] = succeededAt
			}
		}
	}
	for key, succeededAt := range latestSucceeded {
		succeeded[key] = succeededAt
	}
	if activeScan {
		continuation, continuationErr := w.recovery.Scan(passContext, w.scanCursor, webhookDeliveryRecoveryScanPageLimit)
		if continuationErr != nil {
			w.mergePending(deliveries, succeeded)
			w.scanSucceeded = succeeded
			if ctx.Err() == nil {
				w.logger.Warn("GitHub App webhook delivery continuation을 읽지 못했습니다", "error", continuationErr)
			}
			w.redeliverPending(passContext, ctx, now)
			return
		}
		continuationDeliveries := webhookDeliveriesOnOrAfter(continuation.Deliveries, currentAfter)
		deliveries = append(deliveries, continuationDeliveries...)
		for key, succeededAt := range webhookDeliverySuccesses(continuationDeliveries) {
			if current, exists := succeeded[key]; !exists || succeededAt.After(current) {
				succeeded[key] = succeededAt
			}
		}
		w.mergePending(deliveries, succeeded)
		continuationFinished := continuation.NextCursor == "" || webhookDeliveryScanReached(continuation.Deliveries, currentAfter)
		if continuationFinished {
			w.wrapWebhookDeliveryScan(latest, currentAfter, latestSucceeded)
		} else {
			w.scanCursor = continuation.NextCursor
			w.scanSucceeded = succeeded
		}
	} else {
		w.mergePending(deliveries, succeeded)
		w.wrapWebhookDeliveryScan(latest, currentAfter, latestSucceeded)
	}
	w.redeliverPending(passContext, ctx, now)
}

func (w *WebhookDeliveryRecoveryWorker) wrapWebhookDeliveryScan(latest webhookrecovery.ScanResult, deliveredAfter time.Time, succeeded map[string]time.Time) {
	if latest.NextCursor == "" || webhookDeliveryScanReached(latest.Deliveries, deliveredAfter) {
		w.scanCursor = ""
		w.scanSucceeded = map[string]time.Time{}
		return
	}
	w.scanCursor = latest.NextCursor
	w.scanSucceeded = succeeded
}

func (w *WebhookDeliveryRecoveryWorker) mergePending(deliveries []webhookrecovery.Delivery, succeeded map[string]time.Time) {
	latestAttempts := map[string]webhookrecovery.Delivery{}
	for _, delivery := range deliveries {
		key := webhookDeliveryRecoveryKey(delivery)
		if key == "" {
			continue
		}
		latest := latestAttempts[key]
		if latest.ID == 0 || delivery.DeliveredAt.After(latest.DeliveredAt) || delivery.DeliveredAt.Equal(latest.DeliveredAt) && delivery.ID > latest.ID {
			latestAttempts[key] = delivery
		}
	}
	for key := range succeeded {
		delete(w.pending, key)
	}
	for key, delivery := range latestAttempts {
		if _, exists := succeeded[key]; exists || delivery.Status == "OK" {
			continue
		}
		pending := w.pending[key]
		if pending.ID == 0 || delivery.DeliveredAt.After(pending.DeliveredAt) || delivery.DeliveredAt.Equal(pending.DeliveredAt) && delivery.ID > pending.ID {
			w.pending[key] = delivery
		}
	}
}

func (w *WebhookDeliveryRecoveryWorker) redeliverPending(passContext context.Context, ctx context.Context, now time.Time) {
	if passContext.Err() != nil {
		return
	}
	deliveries := w.pendingDeliveries(webhookDeliveryRecoveryCandidateLimit)
	redelivered := 0
	attempts := 0
	for _, delivery := range deliveries {
		if attempts >= webhookDeliveryRecoveryRedeliveryLimit {
			break
		}
		if delivery.DeliveredAt.After(now.Add(-webhookDeliveryRecoveryMinimumAge)) {
			continue
		}
		attemptKey := webhookDeliveryRecoveryKey(delivery)
		if attemptedAt, exists := w.attempted[attemptKey]; exists && attemptedAt.After(now.Add(-webhookDeliveryRecoveryRepeatInterval)) {
			continue
		}
		w.attempted[attemptKey] = now
		attempts++
		callContext, callCancel := context.WithTimeout(passContext, webhookDeliveryRecoveryCallTimeout)
		err := w.recovery.Redeliver(callContext, delivery.ID)
		callCancel()
		if err != nil {
			if ctx.Err() == nil {
				w.logger.Warn("GitHub App webhook delivery를 재전송하지 못했습니다", "delivery", delivery.ID, "event", delivery.Event, "delivery_status", delivery.Status, "status_code", delivery.StatusCode, "error", err)
			}
		} else {
			redelivered++
			delete(w.pending, attemptKey)
			w.logger.Info("GitHub App webhook delivery를 재전송했습니다", "delivery", delivery.ID, "event", delivery.Event, "delivery_status", delivery.Status, "status_code", delivery.StatusCode)
		}
		if err != nil && !delivery.DeliveredAt.After(now.Add(-webhookDeliveryRecoveryLookback)) {
			delete(w.pending, attemptKey)
		}
		if attempts < webhookDeliveryRecoveryRedeliveryLimit {
			select {
			case <-passContext.Done():
				return
			case <-time.After(webhookDeliveryRecoveryRateInterval):
			}
		}
	}
	if redelivered > 0 {
		w.logger.Info("GitHub App webhook delivery 복구 pass를 완료했습니다", "redelivered", redelivered)
	}
}

func (w *WebhookDeliveryRecoveryWorker) pendingDeliveries(limit int) []webhookrecovery.Delivery {
	deliveries := make([]webhookrecovery.Delivery, 0, min(len(w.pending), limit))
	for _, delivery := range w.pending {
		deliveries = append(deliveries, delivery)
	}
	sort.Slice(deliveries, func(left int, right int) bool {
		if deliveries[left].DeliveredAt.Equal(deliveries[right].DeliveredAt) {
			return deliveries[left].ID < deliveries[right].ID
		}
		return deliveries[left].DeliveredAt.Before(deliveries[right].DeliveredAt)
	})
	if len(deliveries) > limit {
		deliveries = deliveries[:limit]
	}
	return deliveries
}

func (w *WebhookDeliveryRecoveryWorker) forgetAttemptsBefore(cutoff time.Time) {
	for deliveryID, attemptedAt := range w.attempted {
		if attemptedAt.Before(cutoff) {
			delete(w.attempted, deliveryID)
		}
	}
}

func (w *WebhookDeliveryRecoveryWorker) forgetPendingBefore(cutoff time.Time) {
	for key, delivery := range w.pending {
		if delivery.DeliveredAt.Before(cutoff) {
			delete(w.pending, key)
		}
	}
}

func webhookDeliveriesOnOrAfter(deliveries []webhookrecovery.Delivery, cutoff time.Time) []webhookrecovery.Delivery {
	filtered := make([]webhookrecovery.Delivery, 0, len(deliveries))
	for _, delivery := range deliveries {
		if delivery.ID <= 0 || delivery.DeliveredAt.IsZero() || delivery.DeliveredAt.Before(cutoff) {
			continue
		}
		filtered = append(filtered, delivery)
	}
	return filtered
}

func webhookDeliverySuccesses(deliveries []webhookrecovery.Delivery) map[string]time.Time {
	succeeded := map[string]time.Time{}
	for _, delivery := range deliveries {
		if delivery.Status != "OK" {
			continue
		}
		if key := webhookDeliveryRecoveryKey(delivery); key != "" {
			if current, exists := succeeded[key]; !exists || delivery.DeliveredAt.After(current) {
				succeeded[key] = delivery.DeliveredAt
			}
		}
	}
	return succeeded
}

func webhookDeliveryScanReached(deliveries []webhookrecovery.Delivery, cutoff time.Time) bool {
	for _, delivery := range deliveries {
		if !delivery.DeliveredAt.IsZero() && delivery.DeliveredAt.Before(cutoff) {
			return true
		}
	}
	return false
}

func webhookDeliveryRecoveryKey(delivery webhookrecovery.Delivery) string {
	if delivery.GUID != "" {
		return delivery.GUID
	}
	if delivery.ID > 0 {
		return strconv.FormatInt(delivery.ID, 10)
	}
	return ""
}
