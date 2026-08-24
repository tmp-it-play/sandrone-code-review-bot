package llm

type Response struct {
	Content        string
	ToolCalls      []ToolCall
	Provider       string
	Model          string
	ModelLabel     string
	FinishReason   string
	Usage          Usage
	ToolExecutions int
}

func (r Response) NeedsToolExecution() bool {
	return len(r.ToolCalls) > 0
}
