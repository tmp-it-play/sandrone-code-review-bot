package mysql

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/mapper"
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
	"gorm.io/gorm"
)

func (r *ReviewExecutionStore) SavePlan(ctx context.Context, runID uint64, runLeaseToken string, units []reviewworkflow.Unit, coverage []reviewworkflow.CoverageItem, plannedAt time.Time) ([]reviewworkflow.Unit, error) {
	stored := make([]reviewworkflow.Unit, 0, len(units))
	err := r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		if err := requireRunLease(transaction, runID, runLeaseToken); err != nil {
			return err
		}
		var existingUnitHashes []string
		if err := transaction.Model(&model.ReviewUnit{}).Where("review_run_id = ?", runID).Pluck("unit_hash", &existingUnitHashes).Error; err != nil {
			return err
		}
		var existingCoverageKeys []string
		if err := transaction.Model(&model.CoverageItem{}).Where("review_run_id = ?", runID).Pluck("coverage_key", &existingCoverageKeys).Error; err != nil {
			return err
		}
		if len(existingUnitHashes) > 0 || len(existingCoverageKeys) > 0 {
			if !samePlan(existingUnitHashes, existingCoverageKeys, units, coverage) {
				return errors.New("이미 저장된 리뷰 계획과 새 계획이 다릅니다")
			}
		} else {
			for _, unit := range units {
				unit.RunID = runID
				entry := mapper.ToReviewUnitModel(unit)
				if err := transaction.Create(&entry).Error; err != nil {
					return err
				}
			}
		}

		var unitEntries []model.ReviewUnit
		if err := transaction.Where("review_run_id = ?", runID).Order("ordinal ASC").Find(&unitEntries).Error; err != nil {
			return err
		}
		unitIDs := make(map[string]uint64, len(unitEntries))
		for _, entry := range unitEntries {
			unitIDs[entry.UnitHash] = entry.ID
			stored = append(stored, mapper.ToReviewUnit(entry))
		}

		if len(existingCoverageKeys) == 0 {
			entries := make([]model.CoverageItem, 0, len(coverage))
			for _, item := range coverage {
				item.RunID = runID
				if item.UnitHash != "" {
					unitID, found := unitIDs[item.UnitHash]
					if !found {
						return fmt.Errorf("coverage unit %s를 찾지 못했습니다", item.UnitHash)
					}
					item.UnitID = &unitID
				}
				entries = append(entries, mapper.ToCoverageItemModel(item))
			}
			if len(entries) > 0 {
				if err := transaction.CreateInBatches(&entries, 100).Error; err != nil {
					return err
				}
			}
		}
		updated := transaction.Model(&model.ReviewRun{}).
			Where("id = ? AND lease_token = ? AND status IN ?", runID, runLeaseToken, []string{string(reviewworkflow.RunStatusPlanning), string(reviewworkflow.RunStatusRunning)}).
			Updates(map[string]any{
				"status":       string(reviewworkflow.RunStatusRunning),
				"heartbeat_at": plannedAt,
			})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return errors.New("종료되었거나 충돌한 리뷰 실행에는 계획을 저장할 수 없습니다")
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("리뷰 실행 계획을 저장하지 못했습니다: %w", err)
	}
	return stored, nil
}

func samePlan(storedUnits []string, storedCoverage []string, units []reviewworkflow.Unit, coverage []reviewworkflow.CoverageItem) bool {
	expectedUnits := make([]string, 0, len(units))
	for _, unit := range units {
		expectedUnits = append(expectedUnits, unit.Hash)
	}
	expectedCoverage := make([]string, 0, len(coverage))
	for _, item := range coverage {
		expectedCoverage = append(expectedCoverage, item.Key)
	}
	sort.Strings(storedUnits)
	sort.Strings(storedCoverage)
	sort.Strings(expectedUnits)
	sort.Strings(expectedCoverage)
	return slices.Equal(storedUnits, expectedUnits) && slices.Equal(storedCoverage, expectedCoverage)
}
