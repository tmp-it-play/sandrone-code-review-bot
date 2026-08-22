package openaicompat

type apiError struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    any    `json:"code"`
	} `json:"error"`
	Message string `json:"message"`
}

func (e apiError) text() string {
	if e.Error.Message != "" {
		return e.Error.Message
	}
	return e.Message
}
