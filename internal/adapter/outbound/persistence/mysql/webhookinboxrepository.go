package mysql

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/mapper"
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/webhookinbox"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type WebhookInboxRepository struct {
	database *gorm.DB
}

func NewWebhookInboxRepository(database *gorm.DB) *WebhookInboxRepository {
	return &WebhookInboxRepository{database: database}
}

func (r *WebhookInboxRepository) Store(ctx context.Context, delivery webhookinbox.Delivery) error {
	delivery, err := initialWebhookDelivery(delivery)
	if err != nil {
		return err
	}
	err = r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		entry := mapper.ToWebhookDeliveryModel(delivery)
		if err := transaction.Clauses(clause.OnConflict{DoNothing: true}).Create(&entry).Error; err != nil {
			return err
		}
		stored, err := matchingWebhookDelivery(transaction, delivery)
		if err != nil || webhookinbox.Status(stored.Status) != webhookinbox.StatusFailed {
			return err
		}
		updated := transaction.Model(&model.WebhookDelivery{}).
			Where("id = ? AND status = ?", stored.ID, string(webhookinbox.StatusFailed)).
			Updates(map[string]any{
				"status":           string(webhookinbox.StatusPending),
				"payload":          delivery.Payload,
				"attempts":         0,
				"lease_token":      "",
				"lease_expires_at": nil,
				"available_at":     delivery.AvailableAt,
				"received_at":      delivery.ReceivedAt,
				"completed_at":     nil,
				"expires_at":       nil,
				"last_error":       "",
			})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return webhookinbox.ErrIdentityConflict
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("웹훅 delivery를 저장하거나 검증하지 못했습니다: %w", err)
	}
	return nil
}

func (r *WebhookInboxRepository) ClaimNext(ctx context.Context, now time.Time, leaseExpiresAt time.Time) (webhookinbox.Delivery, bool, error) {
	if now.IsZero() || !leaseExpiresAt.After(now) {
		return webhookinbox.Delivery{}, false, fmt.Errorf("%w: 유효한 webhook lease 시간이 필요합니다", webhookinbox.ErrInvalidDelivery)
	}
	leaseToken, err := newLeaseToken()
	if err != nil {
		return webhookinbox.Delivery{}, false, fmt.Errorf("웹훅 delivery lease를 만들지 못했습니다: %w", err)
	}
	var entry model.WebhookDelivery
	found := false
	err = r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		eligibility := "((status = ? AND available_at <= ?) OR (status = ? AND (lease_expires_at IS NULL OR lease_expires_at <= ?)))"
		claimErr := transaction.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where(eligibility, string(webhookinbox.StatusPending), now, string(webhookinbox.StatusProcessing), now).
			Order("available_at ASC").
			Order("received_at ASC").
			Order("id ASC").
			First(&entry).Error
		if errors.Is(claimErr, gorm.ErrRecordNotFound) {
			return nil
		}
		if claimErr != nil {
			return claimErr
		}
		updated := transaction.Model(&model.WebhookDelivery{}).
			Where("id = ?", entry.ID).
			Where(eligibility, string(webhookinbox.StatusPending), now, string(webhookinbox.StatusProcessing), now).
			Updates(map[string]any{
				"status":           string(webhookinbox.StatusProcessing),
				"attempts":         gorm.Expr("attempts + 1"),
				"lease_token":      leaseToken,
				"lease_expires_at": leaseExpiresAt,
			})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return webhookinbox.ErrLeaseLost
		}
		entry.Status = string(webhookinbox.StatusProcessing)
		entry.Attempts++
		entry.LeaseToken = leaseToken
		entry.LeaseExpiresAt = &leaseExpiresAt
		found = true
		return nil
	})
	if err != nil {
		return webhookinbox.Delivery{}, false, fmt.Errorf("다음 웹훅 delivery를 선점하지 못했습니다: %w", err)
	}
	if !found {
		return webhookinbox.Delivery{}, false, nil
	}
	return mapper.ToWebhookDelivery(entry), true, nil
}

func (r *WebhookInboxRepository) Complete(ctx context.Context, id uint64, leaseToken string, completedAt time.Time, expiresAt time.Time) error {
	if id == 0 || leaseToken == "" || completedAt.IsZero() {
		return fmt.Errorf("%w: 완료할 webhook lease가 올바르지 않습니다", webhookinbox.ErrInvalidDelivery)
	}
	minimumExpiresAt := completedAt.Add(webhookinbox.MinimumCompletedRetention)
	if expiresAt.Before(minimumExpiresAt) {
		expiresAt = minimumExpiresAt
	}
	updated := r.database.WithContext(ctx).Model(&model.WebhookDelivery{}).
		Where("id = ? AND status = ? AND lease_token = ?", id, string(webhookinbox.StatusProcessing), leaseToken).
		Updates(map[string]any{
			"status":           string(webhookinbox.StatusCompleted),
			"payload":          nil,
			"lease_token":      "",
			"lease_expires_at": nil,
			"completed_at":     completedAt,
			"expires_at":       expiresAt,
			"last_error":       "",
		})
	if updated.Error != nil {
		return fmt.Errorf("웹훅 delivery 완료를 저장하지 못했습니다: %w", updated.Error)
	}
	if updated.RowsAffected != 1 {
		return fmt.Errorf("웹훅 delivery 완료 lease가 만료되었습니다: %w", webhookinbox.ErrLeaseLost)
	}
	return nil
}

func (r *WebhookInboxRepository) Retry(ctx context.Context, id uint64, leaseToken string, availableAt time.Time, lastError string) error {
	if id == 0 || leaseToken == "" || availableAt.IsZero() {
		return fmt.Errorf("%w: 재시도할 webhook lease가 올바르지 않습니다", webhookinbox.ErrInvalidDelivery)
	}
	updated := r.database.WithContext(ctx).Model(&model.WebhookDelivery{}).
		Where("id = ? AND status = ? AND lease_token = ?", id, string(webhookinbox.StatusProcessing), leaseToken).
		Updates(map[string]any{
			"status":           string(webhookinbox.StatusPending),
			"lease_token":      "",
			"lease_expires_at": nil,
			"available_at":     availableAt,
			"last_error":       boundedWebhookError(lastError),
		})
	if updated.Error != nil {
		return fmt.Errorf("웹훅 delivery 재시도를 저장하지 못했습니다: %w", updated.Error)
	}
	if updated.RowsAffected != 1 {
		return fmt.Errorf("웹훅 delivery 재시도 lease가 만료되었습니다: %w", webhookinbox.ErrLeaseLost)
	}
	return nil
}

func (r *WebhookInboxRepository) Reject(ctx context.Context, id uint64, leaseToken string, rejectedAt time.Time, expiresAt time.Time, lastError string) error {
	if id == 0 || leaseToken == "" || rejectedAt.IsZero() || !expiresAt.After(rejectedAt) {
		return fmt.Errorf("%w: 거부할 webhook lease와 보존 시간이 올바르지 않습니다", webhookinbox.ErrInvalidDelivery)
	}
	updated := r.database.WithContext(ctx).Model(&model.WebhookDelivery{}).
		Where("id = ? AND status = ? AND lease_token = ?", id, string(webhookinbox.StatusProcessing), leaseToken).
		Updates(map[string]any{
			"status":           string(webhookinbox.StatusFailed),
			"payload":          nil,
			"lease_token":      "",
			"lease_expires_at": nil,
			"completed_at":     rejectedAt,
			"expires_at":       expiresAt,
			"last_error":       boundedWebhookError(lastError),
		})
	if updated.Error != nil {
		return fmt.Errorf("웹훅 delivery 거부를 저장하지 못했습니다: %w", updated.Error)
	}
	if updated.RowsAffected != 1 {
		return fmt.Errorf("웹훅 delivery 거부 lease가 만료되었습니다: %w", webhookinbox.ErrLeaseLost)
	}
	return nil
}

func (r *WebhookInboxRepository) DeleteExpired(ctx context.Context, now time.Time, limit int) (int64, error) {
	if now.IsZero() {
		return 0, fmt.Errorf("%w: 만료 기준 시간이 필요합니다", webhookinbox.ErrInvalidDelivery)
	}
	if limit <= 0 {
		limit = 100
	}
	deleted := int64(0)
	err := r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		ids := make([]uint64, 0, limit)
		if err := transaction.Model(&model.WebhookDelivery{}).
			Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status IN ? AND expires_at IS NOT NULL AND expires_at <= ?", terminalWebhookStatuses(), now).
			Order("expires_at ASC").
			Order("id ASC").
			Limit(limit).
			Pluck("id", &ids).Error; err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		result := transaction.Where("id IN ? AND status IN ? AND expires_at IS NOT NULL AND expires_at <= ?", ids, terminalWebhookStatuses(), now).
			Delete(&model.WebhookDelivery{})
		if result.Error != nil {
			return result.Error
		}
		deleted = result.RowsAffected
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("만료된 웹훅 delivery를 삭제하지 못했습니다: %w", err)
	}
	return deleted, nil
}

func matchingWebhookDelivery(database *gorm.DB, delivery webhookinbox.Delivery) (model.WebhookDelivery, error) {
	query := database.Where("request_identity = ?", delivery.RequestIdentity)
	if delivery.Key != "" {
		query = query.Where("delivery_key = ?", delivery.Key)
	}
	var entry model.WebhookDelivery
	if err := query.Clauses(clause.Locking{Strength: "UPDATE"}).Take(&entry).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return model.WebhookDelivery{}, fmt.Errorf("delivery identity pair가 기존 행과 일치하지 않습니다: %w", webhookinbox.ErrIdentityConflict)
	} else if err != nil {
		return model.WebhookDelivery{}, fmt.Errorf("중복 웹훅 delivery를 확인하지 못했습니다: %w", err)
	}
	if entry.EventType != delivery.EventType || !strings.EqualFold(entry.PayloadHash, delivery.PayloadHash) {
		return model.WebhookDelivery{}, fmt.Errorf("delivery identity가 다른 이벤트 본문에 사용되었습니다: %w", webhookinbox.ErrIdentityConflict)
	}
	return entry, nil
}

func initialWebhookDelivery(delivery webhookinbox.Delivery) (webhookinbox.Delivery, error) {
	if delivery.RequestIdentity == "" || delivery.EventType == "" || delivery.ReceivedAt.IsZero() {
		return webhookinbox.Delivery{}, fmt.Errorf("%w: event type, request identity, received time이 필요합니다", webhookinbox.ErrInvalidDelivery)
	}
	digest := sha256.Sum256(delivery.Payload)
	payloadHash := hex.EncodeToString(digest[:])
	if delivery.PayloadHash != "" && !strings.EqualFold(delivery.PayloadHash, payloadHash) {
		return webhookinbox.Delivery{}, fmt.Errorf("%w: payload hash가 본문과 일치하지 않습니다", webhookinbox.ErrInvalidDelivery)
	}
	delivery.ID = 0
	delivery.PayloadHash = payloadHash
	delivery.Status = webhookinbox.StatusPending
	delivery.Attempts = 0
	delivery.LeaseToken = ""
	delivery.LeaseExpiresAt = nil
	delivery.CompletedAt = nil
	delivery.ExpiresAt = nil
	delivery.LastError = ""
	if delivery.AvailableAt.IsZero() {
		delivery.AvailableAt = delivery.ReceivedAt
	}
	return delivery, nil
}

func boundedWebhookError(value string) string {
	trimmed := strings.TrimSpace(value)
	runes := []rune(trimmed)
	if len(runes) <= 4000 {
		return trimmed
	}
	return string(runes[:4000])
}

func terminalWebhookStatuses() []string {
	return []string{string(webhookinbox.StatusCompleted), string(webhookinbox.StatusFailed)}
}
