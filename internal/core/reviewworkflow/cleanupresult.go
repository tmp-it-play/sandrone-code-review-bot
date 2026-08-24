package reviewworkflow

type CleanupResult struct {
	Runs         int64
	Reviews      int64
	Findings     int64
	Occurrences  int64
	States       int64
	Publications int64
}
