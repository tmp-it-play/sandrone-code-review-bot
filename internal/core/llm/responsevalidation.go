package llm

type ResponseValidation struct {
	Policy        string
	RequiredPaths []string
}

const ResponseValidationReviewResult = "review-result-v1"

const ResponseValidationSummaryResult = "summary-result-v1"
