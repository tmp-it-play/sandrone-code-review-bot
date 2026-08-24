package llm

type ToolCall struct {
	ID               string
	Name             string
	Arguments        string
	ThoughtSignature string
}
