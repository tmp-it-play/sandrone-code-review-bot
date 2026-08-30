package reviewworkflow

type RunAnchor struct {
	BaseSHA            string
	HeadSHA            string
	ReplacementPending bool
}
