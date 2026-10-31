package reviewpullrequest

import (
	"fmt"

	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

const maximumAdaptiveReviewDepth = 4

type reviewUnitWorkPlanner struct {
	filesByPath   map[string]pullrequest.ChangedFile
	coverage      []reviewworkflow.CoverageItem
	failedParents map[string]string
}

func newReviewUnitWorkPlanner(files []pullrequest.ChangedFile, coverage []reviewworkflow.CoverageItem, splitUnits []reviewworkflow.Unit) *reviewUnitWorkPlanner {
	filesByPath := map[string]pullrequest.ChangedFile{}
	for _, file := range files {
		filesByPath[file.Path] = file
	}
	failedParents := make(map[string]string)
	for _, unit := range splitUnits {
		if unit.ErrorSummary != "" {
			failedParents[unit.Hash] = unit.ParentHash
		}
	}
	return &reviewUnitWorkPlanner{filesByPath: filesByPath, coverage: coverage, failedParents: failedParents}
}

func (p *reviewUnitWorkPlanner) Build(units []reviewworkflow.Unit) ([]reviewUnitWork, error) {
	works := make([]reviewUnitWork, 0, len(units))
	for _, unit := range units {
		work, err := p.work(unit)
		if err != nil {
			return nil, err
		}
		works = append(works, work)
	}
	return works, nil
}

func (p *reviewUnitWorkPlanner) Split(work reviewUnitWork, policy reviewRoutePolicy) (reviewUnitSplit, bool, error) {
	if len(work.files) < 2 || p.refinementDepth(work.unit) >= maximumAdaptiveReviewDepth {
		return reviewUnitSplit{}, false, nil
	}
	left, right, found := p.partition(work, policy)
	if !found {
		return reviewUnitSplit{}, false, nil
	}
	split, err := p.splitGroups(work, left, right)
	return split, err == nil, err
}

func (p *reviewUnitWorkPlanner) refinementDepth(unit reviewworkflow.Unit) int {
	depth := 0
	for parent := unit.ParentHash; parent != "" && depth < maximumAdaptiveReviewDepth; {
		ancestor, failed := p.failedParents[parent]
		if !failed {
			break
		}
		depth++
		parent = ancestor
	}
	return depth
}

func (p *reviewUnitWorkPlanner) splitGroups(work reviewUnitWork, left, right []pullrequest.ChangedFile) (reviewUnitSplit, error) {
	groups := [][]pullrequest.ChangedFile{left, right}
	result := reviewUnitSplit{children: make([]reviewworkflow.Unit, 0, 2)}
	for index, files := range groups {
		paths := filePaths(files)
		keys := p.coverageKeys(work.unit.Hash, paths)
		child, err := reviewworkflow.NewChildUnit(work.unit, index, reviewworkflow.NewUnitSpec(paths, keys))
		if err != nil {
			return reviewUnitSplit{}, err
		}
		result.children = append(result.children, child)
		for _, key := range keys {
			result.assignments = append(result.assignments, reviewworkflow.CoverageAssignment{CoverageKey: key, UnitHash: child.Hash})
		}
	}
	return result, nil
}

func (p *reviewUnitWorkPlanner) ApplySplit(split reviewUnitSplit) {
	assignments := make(map[string]string, len(split.assignments))
	for _, assignment := range split.assignments {
		assignments[assignment.CoverageKey] = assignment.UnitHash
	}
	for index := range p.coverage {
		if unitHash, found := assignments[p.coverage[index].Key]; found && p.coverage[index].Status == reviewworkflow.CoverageStatusPlanned {
			p.coverage[index].UnitHash = unitHash
		}
	}
}

func (p *reviewUnitWorkPlanner) work(unit reviewworkflow.Unit) (reviewUnitWork, error) {
	spec, err := reviewworkflow.ParseUnitSpec(unit.SpecJSON)
	if err != nil {
		return reviewUnitWork{}, err
	}
	files := make([]pullrequest.ChangedFile, 0, len(spec.Paths))
	for _, path := range spec.Paths {
		file, found := p.filesByPath[path]
		if !found {
			return reviewUnitWork{}, fmt.Errorf("리뷰 unit 파일 %s를 현재 계획에서 찾지 못했습니다", path)
		}
		files = append(files, file)
	}
	keySet := make(map[string]struct{}, len(spec.CoverageKeys))
	for _, key := range spec.CoverageKeys {
		keySet[key] = struct{}{}
	}
	for index := range p.coverage {
		if _, found := keySet[p.coverage[index].Key]; found && p.coverage[index].Status == reviewworkflow.CoverageStatusPlanned {
			p.coverage[index].UnitHash = unit.Hash
		}
	}
	return reviewUnitWork{unit: unit, files: (semanticFilePlanner{}).Order(files)}, nil
}

func (p *reviewUnitWorkPlanner) partition(work reviewUnitWork, policy reviewRoutePolicy) ([]pullrequest.ChangedFile, []pullrequest.ChangedFile, bool) {
	preferredLeft, _ := policy.Split(work.files)
	preferred := len(preferredLeft)
	best := 0
	bestDistance := len(work.files)
	for splitAt := 1; splitAt < len(work.files); splitAt++ {
		leftKeys := p.coverageKeys(work.unit.Hash, filePaths(work.files[:splitAt]))
		rightKeys := p.coverageKeys(work.unit.Hash, filePaths(work.files[splitAt:]))
		if len(leftKeys) == 0 || len(rightKeys) == 0 {
			continue
		}
		distance := splitAt - preferred
		if distance < 0 {
			distance = -distance
		}
		if best == 0 || distance < bestDistance {
			best = splitAt
			bestDistance = distance
		}
	}
	if best == 0 {
		return nil, nil, false
	}
	return append([]pullrequest.ChangedFile(nil), work.files[:best]...), append([]pullrequest.ChangedFile(nil), work.files[best:]...), true
}

func (p *reviewUnitWorkPlanner) coverageKeys(unitHash string, paths []string) []string {
	pathSet := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		pathSet[path] = struct{}{}
	}
	keys := make([]string, 0)
	for _, item := range p.coverage {
		if item.UnitHash != unitHash || item.Status != reviewworkflow.CoverageStatusPlanned {
			continue
		}
		if _, found := pathSet[item.Path]; found {
			keys = append(keys, item.Key)
		}
	}
	return keys
}

func filePaths(files []pullrequest.ChangedFile) []string {
	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, file.Path)
	}
	return paths
}
