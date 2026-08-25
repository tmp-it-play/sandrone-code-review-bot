package mysql

import (
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
	"gorm.io/gorm"
)

func loadDescendantLeafEntries(transaction *gorm.DB, runID uint64, parentHash string) ([]model.ReviewUnit, error) {
	var entries []model.ReviewUnit
	if err := transaction.Where("review_run_id = ?", runID).Order("order_key ASC").Order("id ASC").Find(&entries).Error; err != nil {
		return nil, err
	}
	byHash := make(map[string]model.ReviewUnit, len(entries))
	for _, entry := range entries {
		byHash[entry.UnitHash] = entry
	}
	leaves := make([]model.ReviewUnit, 0)
	for _, entry := range entries {
		if reviewworkflow.UnitStatus(entry.Status) == reviewworkflow.UnitStatusSplit || entry.UnitHash == parentHash {
			continue
		}
		if unitDescendsFrom(entry, parentHash, byHash) {
			leaves = append(leaves, entry)
		}
	}
	return leaves, nil
}
