package mysql

import (
	"fmt"

	"gorm.io/gorm"
)

func removeObsoleteMigrationArtifacts(database *gorm.DB) error {
	if err := database.Exec("DROP TABLE IF EXISTS `null_int64`").Error; err != nil {
		return fmt.Errorf("사용하지 않는 마이그레이션 테이블을 제거하지 못했습니다: %w", err)
	}
	var remaining int64
	if err := database.Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?", "null_int64").Row().Scan(&remaining); err != nil {
		return fmt.Errorf("사용하지 않는 마이그레이션 테이블 제거 결과를 확인하지 못했습니다: %w", err)
	}
	if remaining != 0 {
		return fmt.Errorf("사용하지 않는 마이그레이션 테이블이 남아 있습니다")
	}
	return nil
}
