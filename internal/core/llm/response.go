package llm

type Response struct {
	Content      string
	ToolCalls    []ToolCall
	Provider     string
	Model        string
	FinishReason string
}

func (r Response) NeedsToolExecution() bool {
	return len(r.ToolCalls) > 0
}
