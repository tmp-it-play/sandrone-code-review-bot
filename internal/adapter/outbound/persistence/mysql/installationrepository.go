package mysql

import (
	"context"
	"fmt"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/mapper"
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/installation"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type InstallationRepository struct {
	database *gorm.DB
}

func NewInstallationRepository(database *gorm.DB) *InstallationRepository {
	return &InstallationRepository{database: database}
}

func (r *InstallationRepository) Upsert(ctx context.Context, entry installation.Installation) error {
	record := mapper.ToInstallationModel(entry)
	err := r.database.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"account", "account_type", "selection", "updated_at"}),
	}).Create(&record).Error
	if err != nil {
		return fmt.Errorf("설치 정보를 저장하지 못했다: %w", err)
	}
	return nil
}

func (r *InstallationRepository) UpsertRepository(ctx context.Context, entry installation.Repository) error {
	record := mapper.ToRepositoryModel(entry)
	err := r.database.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "owner"}, {Name: "name"}},
		DoUpdates: clause.AssignmentColumns([]string{"installation_id", "private", "updated_at"}),
	}).Create(&record).Error
	if err != nil {
		return fmt.Errorf("저장소 정보를 저장하지 못했다: %w", err)
	}
	return nil
}

func (r *InstallationRepository) Repositories(ctx context.Context) ([]installation.Repository, error) {
	var entries []model.Repository
	if err := r.database.WithContext(ctx).Order("owner ASC, name ASC").Find(&entries).Error; err != nil {
		return nil, fmt.Errorf("저장소 목록을 읽지 못했다: %w", err)
	}
	repositories := make([]installation.Repository, 0, len(entries))
	for _, entry := range entries {
		repositories = append(repositories, mapper.ToRepository(entry))
	}
	return repositories, nil
}
