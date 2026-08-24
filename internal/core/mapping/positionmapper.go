package mapping

import (
	"sort"

	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewanalysis"
)

type PositionMapper struct {
	MaxInline int
}

func (m PositionMapper) Map(findings []review.Finding, files []pullrequest.ChangedFile) []review.Finding {
	findings, _ = (reviewanalysis.EvidenceVerifier{}).Verify(findings, files)
	lines := map[string]pullrequest.CommentableLines{}
	for _, file := range files {
		lines[file.Path] = pullrequest.ParseCommentableLines(file.Patch)
	}
	ordered := make([]review.Finding, len(findings))
	copy(ordered, findings)
	sort.SliceStable(ordered, func(left, right int) bool {
		return ordered[left].Severity.Rank() > ordered[right].Severity.Rank()
	})
	placed := make([]review.Finding, 0, len(ordered))
	inline := 0
	for _, finding := range ordered {
		commentable, known := lines[finding.File]
		if !known || commentable.IsEmpty() {
			continue
		}
		if m.MaxInline > 0 && inline >= m.MaxInline {
			placed = append(placed, finding.WithPlacement(review.PlacementFallback))
			continue
		}
		if commentable.Contains(finding.Line) {
			placed = append(placed, finding.WithPlacement(review.PlacementInline))
			inline++
			continue
		}
	}
	return placed
}
