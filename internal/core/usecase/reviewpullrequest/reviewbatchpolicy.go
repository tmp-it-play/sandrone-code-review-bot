package reviewpullrequest

type reviewBatchPolicy struct {
	MaxOutputTokens      int
	RequiredOutputTokens int
	MaxFindings          int
}
