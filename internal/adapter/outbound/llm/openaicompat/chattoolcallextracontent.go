package openaicompat

type chatToolCallExtraContent struct {
	Google *chatToolCallGoogle `json:"google,omitempty"`
}
