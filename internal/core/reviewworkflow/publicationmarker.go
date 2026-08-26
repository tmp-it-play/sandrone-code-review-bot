package reviewworkflow

func PublicationMarker(runKey string) string {
	return publicationMarkerPrefix + runKey + " -->"
}
