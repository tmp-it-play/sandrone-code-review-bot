package openaicompat

type chatFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}
