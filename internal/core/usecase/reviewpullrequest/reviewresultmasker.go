package reviewpullrequest

import "github.com/it-play/sandrone-code-review-bot/internal/core/review"

func maskReviewResult(result review.Result, mask func(string) string) review.Result {
	if mask == nil {
		return result
	}
	result.Summary.Overview = mask(result.Summary.Overview)
	for index := range result.Summary.Files {
		result.Summary.Files[index].Note = mask(result.Summary.Files[index].Note)
	}
	for index := range result.Findings {
		result.Findings[index].Title = mask(result.Findings[index].Title)
		result.Findings[index].Body = mask(result.Findings[index].Body)
		result.Findings[index].Suggestion = mask(result.Findings[index].Suggestion)
		maskedEvidence := mask(result.Findings[index].Evidence)
		if maskedEvidence != result.Findings[index].Evidence {
			result.Findings[index].Snapped = true
		}
		result.Findings[index].Evidence = maskedEvidence
		result.Findings[index].RootCause = mask(result.Findings[index].RootCause)
		for occurrenceIndex := range result.Findings[index].Occurrences {
			result.Findings[index].Occurrences[occurrenceIndex].Evidence = mask(result.Findings[index].Occurrences[occurrenceIndex].Evidence)
		}
	}
	return result
}
