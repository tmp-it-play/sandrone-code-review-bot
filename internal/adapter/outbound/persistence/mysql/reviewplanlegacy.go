package mysql

import (
	"errors"
	"fmt"
	"sort"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
	"gorm.io/gorm"
)

func adoptLegacyInitialPlan(transaction *gorm.DB, runID uint64, storedUnits []model.ReviewUnit, storedCoverage []model.CoverageItem, expectedUnits []reviewworkflow.Unit, expectedCoverage []reviewworkflow.CoverageItem) error {
	for _, unit := range storedUnits {
		if unit.ParentUnitHash != "" || unit.Status == string(reviewworkflow.UnitStatusSplit) {
			return errors.New("초기 계획 hash가 없는 실행에 분할 unit이 있습니다")
		}
	}
	if !legacyInitialPlanMatches(storedUnits, storedCoverage, expectedUnits, expectedCoverage) {
		if !legacyCoverageStructureMatches(storedCoverage, expectedCoverage) {
			return errors.New("기존 리뷰 계획과 새 계획이 다릅니다")
		}
		if err := transaction.Where("review_run_id = ?", runID).Delete(&model.CoverageItem{}).Error; err != nil {
			return err
		}
		if err := transaction.Where("review_run_id = ?", runID).Delete(&model.ReviewUnit{}).Error; err != nil {
			return err
		}
		if err := createInitialPlan(transaction, runID, expectedUnits, expectedCoverage); err != nil {
			return err
		}
		return transaction.Model(&model.ReviewRun{}).Where("id = ?", runID).Updates(map[string]any{
			"total_coverage":    0,
			"reviewed_coverage": 0,
			"failed_coverage":   0,
			"deferred_coverage": 0,
			"skipped_coverage":  0,
			"error_summary":     "",
		}).Error
	}
	for _, unit := range expectedUnits {
		if err := transaction.Model(&model.ReviewUnit{}).
			Where("review_run_id = ? AND unit_hash = ?", runID, unit.Hash).
			Updates(map[string]any{
				"parent_unit_hash": "",
				"depth":            0,
				"order_key":        unit.OrderKey,
				"spec_json":        unit.SpecJSON,
			}).Error; err != nil {
			return err
		}
	}
	for _, item := range expectedCoverage {
		if err := transaction.Model(&model.CoverageItem{}).
			Where("review_run_id = ? AND coverage_key = ?", runID, item.Key).
			Update("initial_unit_hash", item.UnitHash).Error; err != nil {
			return err
		}
	}
	return nil
}

func legacyCoverageStructureMatches(storedCoverage []model.CoverageItem, expectedCoverage []reviewworkflow.CoverageItem) bool {
	if len(storedCoverage) != len(expectedCoverage) {
		return false
	}
	storedShape := make([]string, 0, len(storedCoverage))
	for _, item := range storedCoverage {
		storedShape = append(storedShape, legacyCoverageShape(item.CoverageKey, "", item.Kind, item.Path, item.PreviousPath, item.FileStatus, item.HunkHash, item.DuplicateOrdinal, item.OldStart, item.OldCount, item.NewStart, item.NewCount, item.Eligibility))
	}
	expectedShape := make([]string, 0, len(expectedCoverage))
	for _, item := range expectedCoverage {
		expectedShape = append(expectedShape, legacyCoverageShape(item.Key, "", string(item.Kind), item.Path, item.PreviousPath, item.FileStatus, item.HunkHash, item.DuplicateOrdinal, item.OldStart, item.OldCount, item.NewStart, item.NewCount, string(item.Eligibility)))
	}
	sort.Strings(storedShape)
	sort.Strings(expectedShape)
	return equalStrings(storedShape, expectedShape)
}

func legacyInitialPlanMatches(storedUnits []model.ReviewUnit, storedCoverage []model.CoverageItem, expectedUnits []reviewworkflow.Unit, expectedCoverage []reviewworkflow.CoverageItem) bool {
	if len(storedUnits) != len(expectedUnits) || len(storedCoverage) != len(expectedCoverage) {
		return false
	}
	storedUnitByID := make(map[uint64]string, len(storedUnits))
	storedUnitShape := make([]string, 0, len(storedUnits))
	for _, unit := range storedUnits {
		storedUnitByID[unit.ID] = unit.UnitHash
		storedUnitShape = append(storedUnitShape, fmt.Sprintf("%s\x00%d\x00%s", unit.UnitHash, unit.Ordinal, unit.Kind))
	}
	expectedUnitShape := make([]string, 0, len(expectedUnits))
	for _, unit := range expectedUnits {
		expectedUnitShape = append(expectedUnitShape, fmt.Sprintf("%s\x00%d\x00%s", unit.Hash, unit.Ordinal, unit.Kind))
	}
	sort.Strings(storedUnitShape)
	sort.Strings(expectedUnitShape)
	if !equalStrings(storedUnitShape, expectedUnitShape) {
		return false
	}
	storedCoverageShape := make([]string, 0, len(storedCoverage))
	for _, item := range storedCoverage {
		unitHash := ""
		if item.ReviewUnitID != nil {
			unitHash = storedUnitByID[*item.ReviewUnitID]
		}
		storedCoverageShape = append(storedCoverageShape, legacyCoverageShape(item.CoverageKey, unitHash, item.Kind, item.Path, item.PreviousPath, item.FileStatus, item.HunkHash, item.DuplicateOrdinal, item.OldStart, item.OldCount, item.NewStart, item.NewCount, item.Eligibility))
	}
	expectedCoverageShape := make([]string, 0, len(expectedCoverage))
	for _, item := range expectedCoverage {
		expectedCoverageShape = append(expectedCoverageShape, legacyCoverageShape(item.Key, item.UnitHash, string(item.Kind), item.Path, item.PreviousPath, item.FileStatus, item.HunkHash, item.DuplicateOrdinal, item.OldStart, item.OldCount, item.NewStart, item.NewCount, string(item.Eligibility)))
	}
	sort.Strings(storedCoverageShape)
	sort.Strings(expectedCoverageShape)
	return equalStrings(storedCoverageShape, expectedCoverageShape)
}

func legacyCoverageShape(key string, unitHash string, kind string, path string, previousPath string, fileStatus string, hunkHash string, duplicateOrdinal int, oldStart int, oldCount int, newStart int, newCount int, eligibility string) string {
	return fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%d\x00%d\x00%d\x00%d\x00%d\x00%s", key, unitHash, kind, path, previousPath, fileStatus, hunkHash, duplicateOrdinal, oldStart, oldCount, newStart, newCount, eligibility)
}

func equalStrings(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
