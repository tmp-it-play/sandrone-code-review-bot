package outbound

import (
	"context"

	"github.com/it-play/sandrone-code-review-bot/internal/core/webhookrecovery"
)

type WebhookDeliveryRecovery interface {
	Scan(ctx context.Context, cursor string, pageLimit int) (webhookrecovery.ScanResult, error)
	Redeliver(ctx context.Context, deliveryID int64) error
}
