package mysql

import (
	"context"
	"fmt"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/mapper"
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/command"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type CommandRepository struct {
	database *gorm.DB
}

func NewCommandRepository(database *gorm.DB) *CommandRepository {
	return &CommandRepository{database: database}
}

func (r *CommandRepository) Record(ctx context.Context, invocation command.Invocation) error {
	entry := mapper.ToCommandModel(invocation)
	if err := r.database.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&entry).Error; err != nil {
		return fmt.Errorf("명령 기록을 저장하지 못했습니다: %w", err)
	}
	return nil
}

func (r *CommandRepository) Recent(ctx context.Context, limit int) ([]command.Invocation, error) {
	if limit <= 0 {
		limit = 50
	}
	var entries []model.CommandInvocation
	if err := r.database.WithContext(ctx).Order("id DESC").Limit(limit).Find(&entries).Error; err != nil {
		return nil, fmt.Errorf("명령 기록을 읽지 못했습니다: %w", err)
	}
	invocations := make([]command.Invocation, 0, len(entries))
	for _, entry := range entries {
		invocations = append(invocations, mapper.ToCommandInvocation(entry))
	}
	return invocations, nil
}
