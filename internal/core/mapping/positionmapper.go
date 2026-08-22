package mapping

import (
	"sort"

	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

type PositionMapper struct {
	MaxInline int
}

func (m PositionMapper) Map(findings []review.Finding, files []pullrequest.ChangedFile) []review.Finding {
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
			placed = append(placed, finding.WithPlacement(review.PlacementFallback))
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
		snapped, ok := commentable.Nearest(finding.Line)
		if !ok {
			placed = append(placed, finding.WithPlacement(review.PlacementFallback))
			continue
		}
		placed = append(placed, finding.WithLine(snapped).WithPlacement(review.PlacementInline))
		inline++
	}
	return placed
}
