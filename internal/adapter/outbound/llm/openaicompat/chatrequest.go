package openaicompat

type chatRequest struct {
	Model               string              `json:"model"`
	Messages            []chatMessage       `json:"messages"`
	Temperature         *float64            `json:"temperature,omitempty"`
	TopP                *float64            `json:"top_p,omitempty"`
	MaxTokens           int                 `json:"max_tokens,omitempty"`
	MaxCompletionTokens int                 `json:"max_completion_tokens,omitempty"`
	Tools               []chatTool          `json:"tools,omitempty"`
	ResponseFormat      *responseFormat     `json:"response_format,omitempty"`
	ReasoningEffort     string              `json:"reasoning_effort,omitempty"`
	Reasoning           *reasoningConfig    `json:"reasoning,omitempty"`
	ChatTemplateKwargs  *chatTemplateKwargs `json:"chat_template_kwargs,omitempty"`
	ParallelToolCalls   *bool               `json:"parallel_tool_calls,omitempty"`
}
