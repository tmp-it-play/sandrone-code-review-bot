package mysql

import "gorm.io/gorm"

type ReviewRetentionRepository struct {
	database *gorm.DB
}

func NewReviewRetentionRepository(database *gorm.DB) *ReviewRetentionRepository {
	return &ReviewRetentionRepository{database: database}
}
