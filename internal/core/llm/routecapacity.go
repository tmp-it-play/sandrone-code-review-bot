package llm

type RouteCapacity struct {
	Provider           string
	Model              string
	PromptChars        int
	ToolCalling        bool
	MaxOutputTokens    int
	UsableOutputTokens int
}
