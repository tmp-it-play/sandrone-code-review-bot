package mysql

import (
	"context"
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
		Provider:         event.Provider,
		Model:            event.Model,
		Role:             event.Role,
		Outcome:          event.Outcome,
		Status:           event.Status,
		PromptTokens:     event.PromptTokens,
		CompletionTokens: event.CompletionTokens,
		TotalTokens:      event.TotalTokens,
		OccurredAt:       event.OccurredAt,
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

func (r *UsageRepository) attachLastFailure(ctx context.Context, byProvider map[string]*usage.Snapshot) error {
	var rows []struct {
		Provider   string
		Outcome    string
		Status     int
		OccurredAt time.Time
	}
	err := r.database.WithContext(ctx).
		Model(&model.ProviderUsage{}).
		Select("provider, outcome, status, occurred_at").
		Where("outcome <> ?", "succeeded").
		Order("occurred_at DESC").
		Limit(500).
		Scan(&rows).Error
	if err != nil {
		return fmt.Errorf("최근 실패 기록을 읽지 못했습니다: %w", err)
	}
	for _, row := range rows {
		snapshot, ok := byProvider[row.Provider]
		if !ok || !snapshot.LastFailureAt.IsZero() {
			continue
		}
		snapshot.LastFailureKind = row.Outcome
		snapshot.LastFailureStatus = row.Status
		snapshot.LastFailureAt = row.OccurredAt
	}
	return nil
}
