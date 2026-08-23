package reviewworkflow

type CoverageSummary struct {
	Total    int
	Reviewed int
	Failed   int
	Deferred int
	Skipped  int
	Pending  int
}

func SummarizeCoverage(items []CoverageItem) CoverageSummary {
	summary := CoverageSummary{}
	for _, item := range items {
		if item.Eligibility == CoverageEligibilityExcluded {
			summary.Skipped++
			continue
		}
		summary.Total++
		switch {
		case item.Status.SatisfiesReview():
			summary.Reviewed++
		case item.Status == CoverageStatusFailed:
			summary.Failed++
		case item.Status == CoverageStatusDeferred:
			summary.Deferred++
		default:
			summary.Pending++
		}
	}
	return summary
}

func (s CoverageSummary) TerminalStatus() RunStatus {
	if s.Total == 0 {
		return RunStatusSkipped
	}
	if s.Reviewed == s.Total {
		return RunStatusComplete
	}
	if s.Reviewed > 0 {
		return RunStatusPartial
	}
	return RunStatusFailed
}
