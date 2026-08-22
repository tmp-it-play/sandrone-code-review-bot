package setting

import "strings"

type SummaryPlacement string

const (
	SummaryPlacementNewComment      SummaryPlacement = "new-comment"
	SummaryPlacementUpdateComment   SummaryPlacement = "update-comment"
	SummaryPlacementPullRequestBody SummaryPlacement = "pr-body"
)

func ParseSummaryPlacement(value string) (SummaryPlacement, bool) {
	switch SummaryPlacement(strings.ToLower(strings.TrimSpace(value))) {
	case SummaryPlacementNewComment:
		return SummaryPlacementNewComment, true
	case SummaryPlacementUpdateComment:
		return SummaryPlacementUpdateComment, true
	case SummaryPlacementPullRequestBody:
		return SummaryPlacementPullRequestBody, true
	default:
		return "", false
	}
}
