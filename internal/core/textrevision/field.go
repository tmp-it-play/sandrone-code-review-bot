package textrevision

type Field struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Text    string `json:"text"`
	Context string `json:"context,omitempty"`
	Limit   int    `json:"limit"`
}
