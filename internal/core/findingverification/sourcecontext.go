package findingverification

import (
	"sort"

	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

const MaxSourceBytes = 16000

func EvidenceFiles(candidates []review.Finding, files []pullrequest.ChangedFile, budget int) []pullrequest.ChangedFile {
	needed := make(map[string]bool)
	direct := make(map[string]bool)
	for _, f := range candidates {
		needed[f.File] = needed[f.File] || f.RequiresFileScope
		direct[f.File] = direct[f.File] || !f.RequiresFileScope
	}
	selected := make([]pullrequest.ChangedFile, 0, len(needed))
	for _, file := range files {
		if _, found := needed[file.Path]; found {
			selected = append(selected, file)
		}
	}
	sort.Slice(selected, func(i, j int) bool {
		left, right := selected[i], selected[j]
		if needed[left.Path] != needed[right.Path] {
			return needed[left.Path]
		}
		if left.Truncated != right.Truncated {
			return !left.Truncated
		}
		if len(left.Content) != len(right.Content) {
			return len(left.Content) < len(right.Content)
		}
		return left.Path < right.Path
	})
	remaining := budget
	evidence := selected[:0]
	for _, file := range selected {
		if !direct[file.Path] && (file.SourceScope() != "complete" || len(file.Content) > remaining) {
			continue
		}
		file = file.WithSourceLimit(remaining)
		evidence = append(evidence, file)
		remaining -= len(file.Content)
	}
	return evidence
}
