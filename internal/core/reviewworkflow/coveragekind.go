package reviewworkflow

type CoverageKind string

const (
	CoverageKindHunk       CoverageKind = "hunk"
	CoverageKindFileChange CoverageKind = "file_change"
	CoverageKindPatchGap   CoverageKind = "patch_gap"
)
