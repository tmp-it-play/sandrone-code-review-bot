package mapper

import (
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/webhookinbox"
)

func ToWebhookDeliveryModel(delivery webhookinbox.Delivery) model.WebhookDelivery {
	var key *string
	if delivery.Key != "" {
		value := delivery.Key
		key = &value
	}
	return model.WebhookDelivery{
		ID:              delivery.ID,
		DeliveryKey:     key,
		EventType:       delivery.EventType,
		Payload:         delivery.Payload,
		PayloadHash:     delivery.PayloadHash,
		RequestIdentity: delivery.RequestIdentity,
		Status:          string(delivery.Status),
		Attempts:        delivery.Attempts,
		LeaseToken:      delivery.LeaseToken,
		LeaseExpiresAt:  delivery.LeaseExpiresAt,
		AvailableAt:     delivery.AvailableAt,
		ReceivedAt:      delivery.ReceivedAt,
		CompletedAt:     delivery.CompletedAt,
		ExpiresAt:       delivery.ExpiresAt,
		LastError:       delivery.LastError,
	}
}

func ToWebhookDelivery(entry model.WebhookDelivery) webhookinbox.Delivery {
	key := ""
	if entry.DeliveryKey != nil {
		key = *entry.DeliveryKey
	}
	return webhookinbox.Delivery{
		ID:              entry.ID,
		Key:             key,
		EventType:       entry.EventType,
		Payload:         entry.Payload,
		PayloadHash:     entry.PayloadHash,
		RequestIdentity: entry.RequestIdentity,
		Status:          webhookinbox.Status(entry.Status),
		Attempts:        entry.Attempts,
		LeaseToken:      entry.LeaseToken,
		LeaseExpiresAt:  entry.LeaseExpiresAt,
		AvailableAt:     entry.AvailableAt,
		ReceivedAt:      entry.ReceivedAt,
		CompletedAt:     entry.CompletedAt,
		ExpiresAt:       entry.ExpiresAt,
		LastError:       entry.LastError,
	}
}
