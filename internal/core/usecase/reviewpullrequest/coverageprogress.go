package reviewpullrequest

import (
	"fmt"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

func transitionCoverage(items []reviewworkflow.CoverageItem, unitHash string, status reviewworkflow.CoverageStatus, reviewedAt *time.Time) {
	for index := range items {
		if items[index].UnitHash != unitHash || items[index].Status != reviewworkflow.CoverageStatusPlanned {
			continue
		}
		items[index].Status = status
		items[index].ReviewedAt = reviewedAt
	}
}

func coverageDetail(summary reviewworkflow.CoverageSummary) string {
	return fmt.Sprintf(
		"coverage total=%d reviewed=%d failed=%d deferred=%d skipped=%d pending=%d",
		summary.Total,
		summary.Reviewed,
		summary.Failed,
		summary.Deferred,
		summary.Skipped,
		summary.Pending,
	)
}
