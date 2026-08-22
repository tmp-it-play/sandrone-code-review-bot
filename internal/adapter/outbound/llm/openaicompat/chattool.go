package openaicompat

type chatTool struct {
	Type     string       `json:"type"`
	Function chatFunction `json:"function"`
}
