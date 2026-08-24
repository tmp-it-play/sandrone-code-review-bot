package mysql

import (
	"time"

	"gorm.io/gorm"
)

func databaseTime(transaction *gorm.DB) (time.Time, error) {
	var now time.Time
	if err := transaction.Raw("SELECT CURRENT_TIMESTAMP(6)").Scan(&now).Error; err != nil {
		return time.Time{}, err
	}
	return now, nil
}
