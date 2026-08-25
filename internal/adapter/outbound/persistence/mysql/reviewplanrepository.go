package mysql

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/mapper"
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *ReviewExecutionStore) SavePlan(ctx context.Context, runID uint64, runLeaseToken string, units []reviewworkflow.Unit, coverage []reviewworkflow.CoverageItem, plannedAt time.Time) (reviewworkflow.PlanSnapshot, error) {
	preparedUnits, preparedCoverage, expectedHash, err := prepareInitialPlan(units, coverage)
	if err != nil {
		return reviewworkflow.PlanSnapshot{}, fmt.Errorf("리뷰 실행 계획이 유효하지 않습니다: %w", err)
	}
	stored := reviewworkflow.PlanSnapshot{}
	err = r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		if err := requireRunLease(transaction, runID, runLeaseToken); err != nil {
			return err
		}
		var run model.ReviewRun
		if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, runID).Error; err != nil {
			return err
		}
		unitEntries, coverageEntries, err := loadStoredPlan(transaction, runID)
		if err != nil {
			return err
		}
		if len(unitEntries) == 0 && len(coverageEntries) == 0 {
			if err := createInitialPlan(transaction, runID, preparedUnits, preparedCoverage); err != nil {
				return err
			}
			unitEntries, coverageEntries, err = loadStoredPlan(transaction, runID)
			if err != nil {
				return err
			}
		} else if run.InitialPlanHash == "" {
			if err := adoptLegacyInitialPlan(transaction, runID, unitEntries, coverageEntries, preparedUnits, preparedCoverage); err != nil {
				return err
			}
			unitEntries, coverageEntries, err = loadStoredPlan(transaction, runID)
			if err != nil {
				return err
			}
		}
		storedHash, err := storedInitialPlanHash(unitEntries, coverageEntries)
		if err != nil {
			return err
		}
		if storedHash != expectedHash {
			return errors.New("이미 저장된 리뷰 계획과 새 계획이 다릅니다")
		}
		if run.InitialPlanHash != "" && run.InitialPlanHash != expectedHash {
			return errors.New("리뷰 실행의 초기 계획 hash가 일치하지 않습니다")
		}
		updated := transaction.Model(&model.ReviewRun{}).
			Where("id = ? AND lease_token = ? AND status IN ?", runID, runLeaseToken, []string{string(reviewworkflow.RunStatusPlanning), string(reviewworkflow.RunStatusRunning)}).
			Updates(map[string]any{
				"status":            string(reviewworkflow.RunStatusRunning),
				"heartbeat_at":      plannedAt,
				"initial_plan_hash": expectedHash,
			})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return errors.New("종료되었거나 충돌한 리뷰 실행에는 계획을 저장할 수 없습니다")
		}
		leafEntries, err := loadCurrentLeafEntries(transaction, runID)
		if err != nil {
			return err
		}
		splitEntries, err := loadSplitUnitEntries(transaction, runID)
		if err != nil {
			return err
		}
		stored = reviewworkflow.PlanSnapshot{
			Leaves:     mapReviewUnits(leafEntries),
			SplitUnits: mapReviewUnits(splitEntries),
		}
		return nil
	})
	if err != nil {
		return reviewworkflow.PlanSnapshot{}, fmt.Errorf("리뷰 실행 계획을 저장하지 못했습니다: %w", err)
	}
	return stored, nil
}

func loadStoredPlan(transaction *gorm.DB, runID uint64) ([]model.ReviewUnit, []model.CoverageItem, error) {
	var unitEntries []model.ReviewUnit
	if err := transaction.Where("review_run_id = ?", runID).Order("id ASC").Find(&unitEntries).Error; err != nil {
		return nil, nil, err
	}
	var coverageEntries []model.CoverageItem
	if err := transaction.Where("review_run_id = ?", runID).Order("id ASC").Find(&coverageEntries).Error; err != nil {
		return nil, nil, err
	}
	return unitEntries, coverageEntries, nil
}

func createInitialPlan(transaction *gorm.DB, runID uint64, units []reviewworkflow.Unit, coverage []reviewworkflow.CoverageItem) error {
	unitIDs := make(map[string]uint64, len(units))
	for _, unit := range units {
		unit.RunID = runID
		entry := mapper.ToReviewUnitModel(unit)
		if err := transaction.Create(&entry).Error; err != nil {
			return err
		}
		unitIDs[unit.Hash] = entry.ID
	}
	entries := make([]model.CoverageItem, 0, len(coverage))
	for _, item := range coverage {
		item.RunID = runID
		item.InitialUnitHash = item.UnitHash
		if item.UnitHash != "" {
			unitID, found := unitIDs[item.UnitHash]
			if !found {
				return fmt.Errorf("coverage unit %s를 찾지 못했습니다", item.UnitHash)
			}
			item.UnitID = &unitID
		}
		entries = append(entries, mapper.ToCoverageItemModel(item))
	}
	if len(entries) == 0 {
		return nil
	}
	return transaction.CreateInBatches(&entries, 100).Error
}

func loadCurrentLeafEntries(transaction *gorm.DB, runID uint64) ([]model.ReviewUnit, error) {
	var entries []model.ReviewUnit
	err := transaction.Where("review_run_id = ? AND status <> ?", runID, string(reviewworkflow.UnitStatusSplit)).
		Order("order_key ASC").Order("ordinal ASC").Order("id ASC").Find(&entries).Error
	return entries, err
}

func loadSplitUnitEntries(transaction *gorm.DB, runID uint64) ([]model.ReviewUnit, error) {
	var entries []model.ReviewUnit
	err := transaction.Where("review_run_id = ? AND status = ?", runID, string(reviewworkflow.UnitStatusSplit)).
		Order("order_key ASC").Order("ordinal ASC").Order("id ASC").Find(&entries).Error
	return entries, err
}

func mapReviewUnits(entries []model.ReviewUnit) []reviewworkflow.Unit {
	units := make([]reviewworkflow.Unit, 0, len(entries))
	for _, entry := range entries {
		units = append(units, mapper.ToReviewUnit(entry))
	}
	return units
}
