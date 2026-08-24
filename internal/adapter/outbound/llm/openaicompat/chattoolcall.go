package openaicompat

type chatToolCall struct {
	ID           string                    `json:"id"`
	Type         string                    `json:"type"`
	Function     chatFunctionCall          `json:"function"`
	ExtraContent *chatToolCallExtraContent `json:"extra_content,omitempty"`
}
