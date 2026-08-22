package webhook

import (
	"log/slog"
	"net/http"

	gh "github.com/google/go-github/v90/github"
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/observability"
	"github.com/it-play/sandrone-code-review-bot/internal/core/port/outbound"
)

type Handler struct {
	secret       []byte
	router       *EventRouter
	deduplicator outbound.Deduplicator
	metrics      *observability.Metrics
	logger       *slog.Logger
	retention    Retention
}

func NewHandler(secret string, router *EventRouter, deduplicator outbound.Deduplicator, metrics *observability.Metrics, logger *slog.Logger, retention Retention) *Handler {
	return &Handler{
		secret:       []byte(secret),
		router:       router,
		deduplicator: deduplicator,
		metrics:      metrics,
		logger:       logger,
		retention:    retention,
	}
}

func (h *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		http.Error(writer, "POST만 허용한다", http.StatusMethodNotAllowed)
		return
	}
	payload, err := gh.ValidatePayload(request, h.secret)
	if err != nil {
		h.logger.Warn("웹훅 서명 검증에 실패했다", "error", err)
		http.Error(writer, "서명이 올바르지 않다", http.StatusUnauthorized)
		return
	}
	eventType := gh.WebHookType(request)
	deliveryID := gh.DeliveryID(request)
	if deliveryID != "" {
		first, dedupeErr := h.deduplicator.FirstSeen(request.Context(), deliveryID, h.retention.Duration())
		if dedupeErr != nil {
			h.logger.Warn("중복 확인에 실패했다", "delivery", deliveryID, "error", dedupeErr)
		} else if !first {
			writer.WriteHeader(http.StatusOK)
			return
		}
	}
	event, err := gh.ParseWebHook(eventType, payload)
	if err != nil {
		h.logger.Warn("웹훅 본문을 해석하지 못했다", "event", eventType, "error", err)
		writer.WriteHeader(http.StatusAccepted)
		return
	}
	action := h.router.Route(request.Context(), event)
	h.metrics.ObserveWebhook(eventType, action)
	writer.WriteHeader(http.StatusAccepted)
}
