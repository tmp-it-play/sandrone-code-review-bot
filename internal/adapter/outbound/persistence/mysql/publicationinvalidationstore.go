package mysql

import "gorm.io/gorm"

type PublicationInvalidationStore struct {
	database *gorm.DB
}

func NewPublicationInvalidationStore(database *gorm.DB) *PublicationInvalidationStore {
	return &PublicationInvalidationStore{database: database}
}
