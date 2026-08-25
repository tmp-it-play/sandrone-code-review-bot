package reviewworkflow

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"

	"github.com/it-play/sandrone-code-review-bot/internal/core/batching"
	"github.com/it-play/sandrone-code-review-bot/internal/core/diff"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/selection"
)

type PlanBuilder struct {
	MaxFileChars  int
	ExpectedFiles int
}

func (b PlanBuilder) Build(files []pullrequest.ChangedFile, chosen selection.Selection, plan batching.Plan) ([]Unit, []CoverageItem) {
	classifications := b.classifications(files, chosen, plan)
	itemsByPath := make(map[string][]CoverageItem, len(files))
	for _, file := range files {
		classification := classifications[file.Path]
		itemsByPath[file.Path] = b.items(file, classification)
	}

	units := make([]Unit, 0, len(plan.Batches))
	for index, batch := range plan.Batches {
		keys := make([]string, 0)
		paths := make([]string, 0, len(batch))
		for _, file := range batch {
			paths = append(paths, file.Path)
			for _, item := range itemsByPath[file.Path] {
				if item.Status == CoverageStatusPlanned {
					keys = append(keys, item.Key)
				}
			}
		}
		if len(keys) == 0 {
			continue
		}
		sort.Strings(keys)
		spec := NewUnitSpec(paths, keys)
		unitHash := RootUnitHash(spec)
		specJSON := spec.JSON()
		units = append(units, Unit{
			Hash:     unitHash,
			Ordinal:  index + 1,
			Depth:    0,
			OrderKey: RootUnitOrderKey(index + 1),
			Kind:     "semantic_batch",
			SpecJSON: specJSON,
			Status:   UnitStatusPending,
		})
		for _, file := range batch {
			pathItems := itemsByPath[file.Path]
			for itemIndex := range pathItems {
				if pathItems[itemIndex].Status == CoverageStatusPlanned {
					pathItems[itemIndex].UnitHash = unitHash
				}
			}
			itemsByPath[file.Path] = pathItems
		}
	}

	items := make([]CoverageItem, 0)
	for _, file := range files {
		items = append(items, itemsByPath[file.Path]...)
	}
	if b.ExpectedFiles > len(files) {
		items = append(items, CoverageItem{
			Key:         hashParts("coverage-manifest-gap-v1", strconv.Itoa(b.ExpectedFiles), strconv.Itoa(len(files))),
			Kind:        CoverageKindPatchGap,
			Eligibility: CoverageEligibilityUnresolved,
			Status:      CoverageStatusDeferred,
			Reason:      "file_manifest_incomplete",
		})
	}
	return units, items
}

func (b PlanBuilder) classifications(files []pullrequest.ChangedFile, chosen selection.Selection, plan batching.Plan) map[string]coverageClassification {
	classifications := make(map[string]coverageClassification, len(files))
	for _, excluded := range chosen.Excluded {
		classification := coverageClassification{eligibility: CoverageEligibilityExcluded, status: CoverageStatusSkipped, reason: string(excluded.Reason)}
		if excluded.Reason == selection.ExclusionReasonPatchUnavailable {
			classification = coverageClassification{eligibility: CoverageEligibilityUnresolved, status: CoverageStatusDeferred, reason: string(excluded.Reason)}
		}
		classifications[excluded.File.Path] = classification
	}
	for _, file := range chosen.Skipped {
		classifications[file.Path] = coverageClassification{eligibility: CoverageEligibilityEligible, status: CoverageStatusDeferred, reason: "max_files"}
	}
	for _, file := range plan.Overflow {
		classifications[file.Path] = coverageClassification{eligibility: CoverageEligibilityEligible, status: CoverageStatusDeferred, reason: "max_batches"}
	}
	for _, file := range plan.Oversized {
		classifications[file.Path] = coverageClassification{eligibility: CoverageEligibilityEligible, status: CoverageStatusDeferred, reason: "prompt_limit"}
	}
	for _, batch := range plan.Batches {
		for _, file := range batch {
			classification := coverageClassification{eligibility: CoverageEligibilityEligible, status: CoverageStatusPlanned}
			if file.PatchTruncated {
				classification = coverageClassification{eligibility: CoverageEligibilityUnresolved, status: CoverageStatusDeferred, reason: "patch_truncated"}
			}
			classifications[file.Path] = classification
		}
	}
	for _, file := range files {
		if _, found := classifications[file.Path]; !found {
			classifications[file.Path] = coverageClassification{eligibility: CoverageEligibilityUnresolved, status: CoverageStatusDeferred, reason: "unplanned"}
		}
	}
	return classifications
}

func (b PlanBuilder) items(file pullrequest.ChangedFile, classification coverageClassification) []CoverageItem {
	if classification.eligibility != CoverageEligibilityExcluded && b.MaxFileChars > 0 && len(file.Patch) > b.MaxFileChars {
		classification = coverageClassification{eligibility: CoverageEligibilityUnresolved, status: CoverageStatusDeferred, reason: "patch_truncated"}
	}
	hunks := (diff.Parser{}).Parse(file.Patch)
	if len(hunks) == 0 {
		kind := CoverageKindFileChange
		if classification.eligibility == CoverageEligibilityUnresolved {
			kind = CoverageKindPatchGap
		}
		return []CoverageItem{b.placeholder(file, kind, classification)}
	}

	items := make([]CoverageItem, 0, len(hunks)+1)
	added := 0
	deleted := 0
	patchComplete := true
	for _, hunk := range hunks {
		added += hunk.AddedLines
		deleted += hunk.DeletedLines
		itemClassification := classification
		if classification.eligibility != CoverageEligibilityExcluded && !hunk.Complete {
			itemClassification = coverageClassification{eligibility: CoverageEligibilityUnresolved, status: CoverageStatusDeferred, reason: "incomplete_hunk"}
			patchComplete = false
		}
		items = append(items, CoverageItem{
			Key:              coverageKey(file, hunk.Hash, hunk.DuplicateOrdinal),
			Kind:             CoverageKindHunk,
			Path:             file.Path,
			PreviousPath:     file.PreviousPath,
			FileStatus:       file.Status,
			HunkHash:         hunk.Hash,
			DuplicateOrdinal: hunk.DuplicateOrdinal,
			OldStart:         hunk.OldStart,
			OldCount:         hunk.OldCount,
			NewStart:         hunk.NewStart,
			NewCount:         hunk.NewCount,
			Eligibility:      itemClassification.eligibility,
			Status:           itemClassification.status,
			Reason:           itemClassification.reason,
		})
	}
	if added != file.Additions || deleted != file.Deletions {
		patchComplete = false
	}
	if classification.eligibility != CoverageEligibilityExcluded && !patchComplete {
		items = append(items, b.placeholder(file, CoverageKindPatchGap, coverageClassification{
			eligibility: CoverageEligibilityUnresolved,
			status:      CoverageStatusDeferred,
			reason:      "patch_incomplete",
		}))
	}
	return items
}

func (PlanBuilder) placeholder(file pullrequest.ChangedFile, kind CoverageKind, classification coverageClassification) CoverageItem {
	return CoverageItem{
		Key:          hashParts("coverage-file-v1", file.Path, file.PreviousPath, file.Status, string(kind)),
		Kind:         kind,
		Path:         file.Path,
		PreviousPath: file.PreviousPath,
		FileStatus:   file.Status,
		Eligibility:  classification.eligibility,
		Status:       classification.status,
		Reason:       classification.reason,
	}
}

func coverageKey(file pullrequest.ChangedFile, hunkHash string, ordinal int) string {
	return hashParts("coverage-hunk-v1", file.Path, file.PreviousPath, file.Status, hunkHash, strconv.Itoa(ordinal))
}

func hashParts(parts ...string) string {
	hash := sha256.New()
	for _, part := range parts {
		hash.Write([]byte(strconv.Itoa(len(part))))
		hash.Write([]byte{0})
		hash.Write([]byte(part))
	}
	return hex.EncodeToString(hash.Sum(nil))
}
