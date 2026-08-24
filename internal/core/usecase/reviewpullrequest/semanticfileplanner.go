package reviewpullrequest

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewanalysis"
)

type semanticFilePlanner struct{}

func (semanticFilePlanner) Order(files []pullrequest.ChangedFile) []pullrequest.ChangedFile {
	ordered := append([]pullrequest.ChangedFile(nil), files...)
	classifier := reviewanalysis.SpecialistRiskClassifier{}
	sort.SliceStable(ordered, func(left, right int) bool {
		leftRisk := classifier.Score(ordered[left])
		rightRisk := classifier.Score(ordered[right])
		if leftRisk != rightRisk {
			return leftRisk > rightRisk
		}
		leftGroup := semanticFileGroup(ordered[left].Path)
		rightGroup := semanticFileGroup(ordered[right].Path)
		if leftGroup != rightGroup {
			return leftGroup < rightGroup
		}
		return ordered[left].Path < ordered[right].Path
	})
	return ordered
}

func semanticFileGroup(path string) string {
	normalized := filepath.ToSlash(strings.ToLower(path))
	directory := filepath.ToSlash(filepath.Dir(normalized))
	directory = strings.ReplaceAll(directory, "/tests/", "/")
	directory = strings.ReplaceAll(directory, "/test/", "/")
	directory = strings.TrimSuffix(directory, "/tests")
	directory = strings.TrimSuffix(directory, "/test")
	return directory
}
