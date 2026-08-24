package mysql

import "gorm.io/gorm"

type ReviewVerificationStore struct {
	database *gorm.DB
}

func NewReviewVerificationStore(database *gorm.DB) *ReviewVerificationStore {
	return &ReviewVerificationStore{database: database}
}
