package selection

type ExclusionReason string

const (
	ExclusionReasonPathPolicy       ExclusionReason = "path_policy"
	ExclusionReasonNoContentChange  ExclusionReason = "no_content_change"
	ExclusionReasonPatchUnavailable ExclusionReason = "patch_unavailable"
)
