package mysql

import "gorm.io/gorm"

type ReviewPublicationStore struct {
	database *gorm.DB
}

func NewReviewPublicationStore(database *gorm.DB) *ReviewPublicationStore {
	return &ReviewPublicationStore{database: database}
}
