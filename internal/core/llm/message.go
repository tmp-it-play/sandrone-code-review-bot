package llm

type Message struct {
	Role       Role
	Content    string
	ToolCallID string
	ToolCalls  []ToolCall
}
