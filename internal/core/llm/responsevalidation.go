package llm

type ResponseValidation struct {
	Policy        string
	RequiredPaths []string
	Validator     func(string) error `json:"-"`
}

const ResponseValidationReviewResult = "review-result-v1"

const ResponseValidationSummaryResult = "summary-result-v1"

const ResponseValidationNonEmpty = "non-empty-v1"
