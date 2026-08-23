package reviewworkflow

func PublicationMarker(runKey string) string {
	return "<!-- sandrone-review-run:" + runKey + " -->"
}
