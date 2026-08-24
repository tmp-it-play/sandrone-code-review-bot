package github

import (
	"context"
	"fmt"
	"net/http"

	gh "github.com/google/go-github/v90/github"
	"github.com/it-play/sandrone-code-review-bot/internal/core/webhookrecovery"
)

const appWebhookDeliveryPageSize = 100
const appWebhookDeliveryMaxPages = 3

type AppWebhookDeliveryRecovery struct {
	tokens *TokenSource
}

func NewAppWebhookDeliveryRecovery(tokens *TokenSource) *AppWebhookDeliveryRecovery {
	return &AppWebhookDeliveryRecovery{tokens: tokens}
}

func (r *AppWebhookDeliveryRecovery) Scan(ctx context.Context, cursor string, pageLimit int) (webhookrecovery.ScanResult, error) {
	if pageLimit < 1 {
		return webhookrecovery.ScanResult{}, nil
	}
	if pageLimit > appWebhookDeliveryMaxPages {
		pageLimit = appWebhookDeliveryMaxPages
	}
	client, err := r.tokens.AppClient()
	if err != nil {
		return webhookrecovery.ScanResult{}, err
	}
	result := webhookrecovery.ScanResult{}
	options := &gh.ListCursorOptions{Cursor: cursor, PerPage: appWebhookDeliveryPageSize}
	for page := 0; page < pageLimit; page++ {
		deliveries, response, listErr := client.Apps.ListHookDeliveries(ctx, options)
		if listErr != nil {
			return webhookrecovery.ScanResult{}, fmt.Errorf("GitHub App webhook delivery를 읽지 못했습니다: %w", listErr)
		}
		for _, delivery := range deliveries {
			result.Deliveries = append(result.Deliveries, webhookrecovery.Delivery{
				ID:          delivery.GetID(),
				GUID:        delivery.GetGUID(),
				Event:       delivery.GetEvent(),
				Status:      delivery.GetStatus(),
				StatusCode:  delivery.GetStatusCode(),
				DeliveredAt: delivery.GetDeliveredAt().Time,
			})
		}
		if response == nil || response.Cursor == "" {
			result.NextCursor = ""
			break
		}
		result.NextCursor = response.Cursor
		options.Cursor = result.NextCursor
	}
	return result, nil
}

func (r *AppWebhookDeliveryRecovery) Redeliver(ctx context.Context, deliveryID int64) error {
	if deliveryID <= 0 {
		return fmt.Errorf("GitHub App webhook delivery ID가 올바르지 않습니다")
	}
	client, err := r.tokens.AppClient()
	if err != nil {
		return err
	}
	_, response, err := client.Apps.RedeliverHookDelivery(ctx, deliveryID)
	if err != nil {
		return fmt.Errorf("GitHub App webhook delivery를 재전송하지 못했습니다: %w", err)
	}
	if response == nil || response.StatusCode != http.StatusAccepted {
		return fmt.Errorf("GitHub App webhook delivery 재전송 응답이 올바르지 않습니다")
	}
	return nil
}
