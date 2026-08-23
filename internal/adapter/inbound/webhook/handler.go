package webhook

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"time"

	gh "github.com/google/go-github/v90/github"
	"github.com/it-play/sandrone-code-review-bot/internal/core/port/outbound"
	"github.com/it-play/sandrone-code-review-bot/internal/core/webhookinbox"
)

const deliveryStoreTimeout = 5 * time.Second

type Handler struct {
	secret []byte
	inbox  outbound.WebhookInboxRepository
	clock  outbound.Clock
	logger *slog.Logger
}

func NewHandler(secret string, inbox outbound.WebhookInboxRepository, clock outbound.Clock, logger *slog.Logger) *Handler {
	return &Handler{
		secret: []byte(secret),
		inbox:  inbox,
		clock:  clock,
		logger: logger,
	}
}

func (h *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		http.Error(writer, "POST만 허용합니다", http.StatusMethodNotAllowed)
		return
	}
	payload, err := gh.ValidatePayload(request, h.secret)
	if err != nil {
		h.logger.Warn("웹훅 서명 검증에 실패했습니다", "error", err)
		http.Error(writer, "서명이 올바르지 않습니다", http.StatusUnauthorized)
		return
	}
	eventType := gh.WebHookType(request)
	deliveryID := gh.DeliveryID(request)
	identity := webhookRequestIdentity(eventType, deliveryID, payload)
	key := deliveryID
	if key == "" {
		key = identity
	}
	now := h.clock.Now()
	storeContext, cancel := context.WithTimeout(context.WithoutCancel(request.Context()), deliveryStoreTimeout)
	err = h.inbox.Store(storeContext, webhookinbox.Delivery{
		Key:             key,
		EventType:       eventType,
		Payload:         payload,
		RequestIdentity: identity,
		AvailableAt:     now,
		ReceivedAt:      now,
	})
	cancel()
	if err != nil {
		h.logger.Error("웹훅 delivery를 inbox에 저장하지 못했습니다", "event", eventType, "delivery", deliveryID, "error", err)
		status := http.StatusServiceUnavailable
		if errors.Is(err, webhookinbox.ErrIdentityConflict) {
			status = http.StatusConflict
		}
		http.Error(writer, "웹훅 delivery를 저장하지 못했습니다", status)
		return
	}
	writer.WriteHeader(http.StatusAccepted)
}

func webhookRequestIdentity(eventType string, deliveryID string, payload []byte) string {
	if deliveryID != "" {
		return "delivery:" + deliveryID
	}
	hash := sha256.New()
	hash.Write([]byte(eventType))
	hash.Write([]byte{0})
	hash.Write(payload)
	return "payload:" + hex.EncodeToString(hash.Sum(nil))
}
