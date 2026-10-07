package mysql

import (
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/mapper"
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

func initialPlanCoverage(entries []model.CoverageItem) []reviewworkflow.CoverageItem {
	items := make([]reviewworkflow.CoverageItem, 0, len(entries))
	for _, entry := range entries {
		item := mapper.ToCoverageItem(entry)
		item.UnitHash = item.InitialUnitHash
		if item.InitialUnitHash != "" {
			item.Status = reviewworkflow.CoverageStatusPlanned
			item.Reason = ""
			item.ReviewedAt = nil
		}
		items = append(items, item)
	}
	return items
}
