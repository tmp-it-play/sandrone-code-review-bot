package llm

import "strings"

func (r Response) Completed() bool {
	reason := strings.ToLower(strings.TrimSpace(r.FinishReason))
	return reason == "" || reason == "stop"
}
