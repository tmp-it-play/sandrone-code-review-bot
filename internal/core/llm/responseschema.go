package llm

type ResponseSchema struct {
	Name   string         `json:"name"`
	Schema map[string]any `json:"schema"`
}
