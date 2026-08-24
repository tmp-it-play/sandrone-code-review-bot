package mysql

import "gorm.io/gorm"

type ReviewExecutionStore struct {
	database *gorm.DB
}

func NewReviewExecutionStore(database *gorm.DB) *ReviewExecutionStore {
	return &ReviewExecutionStore{database: database}
}
