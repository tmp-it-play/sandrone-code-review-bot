package llm

type RouteCapacity struct {
	Provider           string
	Model              string
	PromptChars        int
	MaxOutputTokens    int
	UsableOutputTokens int
}
