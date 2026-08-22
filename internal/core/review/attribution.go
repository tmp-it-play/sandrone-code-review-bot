package review

type Attribution struct {
	Provider         string
	Model            string
	Label            string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

func (a Attribution) IsEmpty() bool {
	return a.Provider == "" && a.Model == "" && a.Label == ""
}

func (a Attribution) Display() string {
	if a.Label != "" {
		return a.Label
	}
	return a.Model
}

func (a Attribution) HasUsage() bool {
	return a.PromptTokens > 0 || a.CompletionTokens > 0 || a.TotalTokens > 0
}
