package reviewpullrequest

import (
	"context"
	"sort"

	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

const maximumLifecycleContentBytes = 1024 * 1024
const maximumLifecycleContentReads = 32
const maximumLifecycleRevalidations = 500

func (u *UseCase) revalidateFindingOccurrences(ctx context.Context, target pullrequest.Target, files []pullrequest.ChangedFile) error {
	currentPaths := make(map[string]struct{}, len(files))
	removedPaths := make(map[string]struct{}, len(files))
	for _, file := range files {
		if file.Path != "" {
			if file.IsRemoved() {
				removedPaths[file.Path] = struct{}{}
			} else {
				currentPaths[file.Path] = struct{}{}
			}
		}
		if file.PreviousPath != "" && file.PreviousPath != file.Path {
			removedPaths[file.PreviousPath] = struct{}{}
		}
	}
	paths := make([]string, 0, len(currentPaths)+len(removedPaths))
	for path := range currentPaths {
		paths = append(paths, path)
	}
	for path := range removedPaths {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	occurrences, err := u.deps.Findings.OpenOccurrences(ctx, target, paths, maximumLifecycleRevalidations)
	if err != nil || len(occurrences) == 0 {
		return err
	}
	sort.SliceStable(occurrences, func(left int, right int) bool {
		leftOccurrence := occurrences[left]
		rightOccurrence := occurrences[right]
		if leftOccurrence.Path != rightOccurrence.Path {
			return leftOccurrence.Path < rightOccurrence.Path
		}
		if leftOccurrence.RootID != rightOccurrence.RootID {
			return leftOccurrence.RootID < rightOccurrence.RootID
		}
		if leftOccurrence.Evidence != rightOccurrence.Evidence {
			return leftOccurrence.Evidence < rightOccurrence.Evidence
		}
		if leftOccurrence.Line != rightOccurrence.Line {
			return leftOccurrence.Line < rightOccurrence.Line
		}
		return leftOccurrence.ID < rightOccurrence.ID
	})
	type sourceState struct {
		content string
		certain bool
	}
	sources := make(map[string]sourceState)
	usedEvidenceLines := make(map[string]map[int]struct{})
	revalidations := make([]review.OccurrenceRevalidation, 0, len(occurrences))
	contentReads := 0
	for _, occurrence := range occurrences {
		if _, removed := removedPaths[occurrence.Path]; removed {
			revalidations = append(revalidations, review.OccurrenceRevalidation{
				ID:             occurrence.ID,
				SourceReviewID: occurrence.SourceReviewID,
				Resolved:       true,
			})
			continue
		}
		if _, current := currentPaths[occurrence.Path]; !current {
			continue
		}
		state, loaded := sources[occurrence.Path]
		if !loaded {
			if contentReads >= maximumLifecycleContentReads {
				continue
			}
			contentReads++
			content, contentErr := u.deps.Source.FileContent(ctx, target, occurrence.Path, target.HeadSHA)
			state.certain = contentErr == nil && len(content) <= maximumLifecycleContentBytes
			if state.certain {
				state.content = u.deps.Masker.Mask(content)
			}
			sources[occurrence.Path] = state
		}
		revalidation := review.OccurrenceRevalidation{
			ID:             occurrence.ID,
			SourceReviewID: occurrence.SourceReviewID,
		}
		if state.certain && occurrence.Evidence != "" {
			matches := review.OccurrenceEvidenceLines(state.content, occurrence.Evidence)
			if len(matches) == 0 {
				revalidation.Resolved = true
			} else if occurrence.RootID != "" {
				key := occurrence.RootID.String() + "\x1f" + occurrence.Path + "\x1f" + occurrence.Evidence
				used := usedEvidenceLines[key]
				if used == nil {
					used = make(map[int]struct{})
					usedEvidenceLines[key] = used
				}
				if line, found := nearestUnusedOccurrenceLine(matches, occurrence.Line, used); found {
					used[line] = struct{}{}
					lineCount := review.OccurrenceEvidenceLineCount(occurrence.Evidence)
					endLine := 0
					fingerprintEnd := line
					if lineCount > 1 {
						endLine = line + lineCount - 1
						fingerprintEnd = endLine
					}
					revalidation.CurrentID = review.NewAnchoredOccurrenceFingerprint(occurrence.RootID, occurrence.Path, line, fingerprintEnd, occurrence.Evidence)
					revalidation.Line = line
					revalidation.EndLine = endLine
				}
			}
		}
		revalidations = append(revalidations, revalidation)
	}
	return u.deps.Findings.RevalidateOccurrences(ctx, target, revalidations)
}

func nearestUnusedOccurrenceLine(matches []int, expected int, used map[int]struct{}) (int, bool) {
	selected := 0
	distance := 0
	for _, line := range matches {
		if _, found := used[line]; found {
			continue
		}
		candidateDistance := line - expected
		if candidateDistance < 0 {
			candidateDistance = -candidateDistance
		}
		if selected == 0 || candidateDistance < distance || candidateDistance == distance && line < selected {
			selected = line
			distance = candidateDistance
		}
	}
	return selected, selected > 0
}
