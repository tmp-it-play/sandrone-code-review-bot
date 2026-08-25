package mysql

import (
	"fmt"

	"gorm.io/gorm"
)

func backfillCoverageInitialUnitHashes(database *gorm.DB) error {
	result := database.Exec("UPDATE coverage_items AS coverage JOIN review_units AS unit ON unit.id = coverage.review_unit_id SET coverage.initial_unit_hash = unit.unit_hash WHERE coverage.initial_unit_hash = ''")
	if result.Error != nil {
		return fmt.Errorf("기존 coverage 초기 unit 연결을 복원하지 못했습니다: %w", result.Error)
	}
	return nil
}
