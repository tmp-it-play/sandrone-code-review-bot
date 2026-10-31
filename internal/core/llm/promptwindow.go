package llm

type PromptWindow struct {
	Messages             []Message
	Items                int
	MaxOutputTokens      int
	RequiredOutputTokens int
	FitMessages          func(int) ([]Message, bool)
}
