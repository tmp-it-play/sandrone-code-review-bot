package model

import "time"

type FindingOccurrence struct {
	ID                    uint64 `gorm:"primaryKey;autoIncrement"`
	Owner                 string `gorm:"size:100;uniqueIndex:idx_finding_occurrence_target,priority:1;index:idx_finding_occurrence_open_path,priority:1"`
	Repository            string `gorm:"size:150;uniqueIndex:idx_finding_occurrence_target,priority:2;index:idx_finding_occurrence_open_path,priority:2"`
	Number                int    `gorm:"uniqueIndex:idx_finding_occurrence_target,priority:3;index:idx_finding_occurrence_open_path,priority:3"`
	OccurrenceFingerprint string `gorm:"size:64;uniqueIndex:idx_finding_occurrence_target,priority:4"`
	CurrentFingerprint    string `gorm:"size:64;index"`
	RootFingerprint       string `gorm:"size:64;index"`
	Path                  string `gorm:"size:500"`
	PathHash              string `gorm:"size:64;index:idx_finding_occurrence_open_path,priority:5"`
	Line                  int
	EndLine               int
	Evidence              string     `gorm:"type:mediumtext"`
	Status                string     `gorm:"size:20;index:idx_finding_occurrence_open_path,priority:4"`
	SourceReviewID        uint64     `gorm:"not null;index"`
	SourceReview          Review     `gorm:"foreignKey:SourceReviewID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	SourceRunID           *uint64    `gorm:"index"`
	SourceRun             *ReviewRun `gorm:"foreignKey:SourceRunID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	OpenedAt              time.Time
	ResolvedAt            *time.Time
	LastCheckedAt         *time.Time
	ExpiresAt             time.Time `gorm:"index"`
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

func (FindingOccurrence) TableName() string {
	return "finding_occurrences"
}
