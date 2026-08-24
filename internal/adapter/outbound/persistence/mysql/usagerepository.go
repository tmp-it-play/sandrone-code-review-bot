package mysql

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/usage"
	"gorm.io/gorm"
)

type UsageRepository struct {
	database *gorm.DB
}

func NewUsageRepository(database *gorm.DB) *UsageRepository {
	return &UsageRepository{database: database}
}

func (r *UsageRepository) Record(ctx context.Context, event usage.Event) error {
	entry := model.ProviderUsage{
		Provider:                   event.Provider,
		Model:                      event.Model,
		Role:                       event.Role,
		Outcome:                    event.Outcome,
		Status:                     event.Status,
		ProviderErrorCode:          event.ProviderErrorCode,
		RequestElapsedMilliseconds: event.RequestElapsedMilliseconds,
		PromptTokens:               event.PromptTokens,
		CompletionTokens:           event.CompletionTokens,
		TotalTokens:                event.TotalTokens,
		OccurredAt:                 event.OccurredAt,
	}
	if err := r.database.WithContext(ctx).Create(&entry).Error; err != nil {
		return fmt.Errorf("사용량을 저장하지 못했습니다: %w", err)
	}
	return nil
}

func (r *UsageRepository) Snapshot(ctx context.Context) ([]usage.Snapshot, error) {
	var rows []struct {
		Provider   string
		Outcome    string
		Total      int
		LastUsedAt time.Time
	}
	err := r.database.WithContext(ctx).
		Model(&model.ProviderUsage{}).
		Select("provider, outcome, count(*) as total, max(occurred_at) as last_used_at").
		Group("provider, outcome").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("사용량 집계를 읽지 못했습니다: %w", err)
	}
	byProvider := map[string]*usage.Snapshot{}
	for _, row := range rows {
		snapshot, ok := byProvider[row.Provider]
		if !ok {
			snapshot = &usage.Snapshot{Provider: row.Provider}
			byProvider[row.Provider] = snapshot
		}
		switch row.Outcome {
		case "succeeded":
			snapshot.Succeeded += row.Total
		case "quota", "rate_limited":
			snapshot.QuotaBlocked += row.Total
		default:
			snapshot.Failed += row.Total
		}
		if row.LastUsedAt.After(snapshot.LastUsedAt) {
			snapshot.LastUsedAt = row.LastUsedAt
		}
	}
	if err := r.attachLastFailure(ctx, byProvider); err != nil {
		return nil, err
	}
	snapshots := make([]usage.Snapshot, 0, len(byProvider))
	for _, snapshot := range byProvider {
		snapshots = append(snapshots, *snapshot)
	}
	return snapshots, nil
}

func (r *UsageRepository) DeleteExpired(ctx context.Context, before time.Time, limit int) (int64, error) {
	if limit <= 0 {
		limit = 500
	}
	var ids []uint64
	if err := r.database.WithContext(ctx).
		Model(&model.ProviderUsage{}).
		Where("occurred_at <= ?", before).
		Order("occurred_at ASC").
		Order("id ASC").
		Limit(limit).
		Pluck("id", &ids).Error; err != nil {
		return 0, fmt.Errorf("만료된 프로바이더 사용량을 찾지 못했습니다: %w", err)
	}
	if len(ids) == 0 {
		return 0, nil
	}
	deleted := r.database.WithContext(ctx).
		Where("id IN ? AND occurred_at <= ?", ids, before).
		Delete(&model.ProviderUsage{})
	if deleted.Error != nil {
		return 0, fmt.Errorf("만료된 프로바이더 사용량을 제거하지 못했습니다: %w", deleted.Error)
	}
	return deleted.RowsAffected, nil
}

func (r *UsageRepository) attachLastFailure(ctx context.Context, byProvider map[string]*usage.Snapshot) error {
	for provider, snapshot := range byProvider {
		var row model.ProviderUsage
		err := r.database.WithContext(ctx).
			Where("provider = ?", provider).
			Order("occurred_at DESC").
			Order("id DESC").
			First(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return fmt.Errorf("최근 실패 기록을 읽지 못했습니다: %w", err)
		}
		if row.Outcome == "succeeded" {
			continue
		}
		snapshot.LastFailureKind = row.Outcome
		snapshot.LastFailureStatus = row.Status
		snapshot.LastFailureProviderErrorCode = row.ProviderErrorCode
		snapshot.LastFailureRequestElapsedMilliseconds = row.RequestElapsedMilliseconds
		snapshot.LastFailureAt = row.OccurredAt
	}
	return nil
}
