package reviewpullrequest

import (
	"github.com/it-play/sandrone-code-review-bot/internal/core/parsing"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewanalysis"
	"github.com/it-play/sandrone-code-review-bot/internal/core/selection"
)

func reviewGroundingValidator(files []pullrequest.ChangedFile, minimum review.Severity, mask func(string) string) func(string) error {
	return func(content string) error {
		result, _, err := (parsing.ResultParser{}).Parse(content)
		if err != nil {
			return err
		}
		result = maskReviewResult(result, mask)
		candidates := (selection.SeverityFilter{Minimum: minimum}).Apply(result.Findings)
		if len(candidates) == 0 {
			return nil
		}
		verified, _ := (reviewanalysis.EvidenceVerifier{}).Verify(candidates, files)
		if len(verified) == 0 {
			return parsing.ErrAllFindingsUnanchored
		}
		return nil
	}
}
