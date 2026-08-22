package llm

type Request struct {
	Messages        []Message
	Temperature     float64
	MaxOutputTokens int
	Tools           []Tool
	ForceJSON       bool
	Providers       []string
}

func (r Request) WithMessages(messages []Message) Request {
	r.Messages = messages
	return r
}
