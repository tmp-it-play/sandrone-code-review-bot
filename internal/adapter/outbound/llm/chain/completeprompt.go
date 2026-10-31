package chain

import "github.com/it-play/sandrone-code-review-bot/internal/core/llm"

func fitCompletePrompt(request llm.Request, messages []llm.Message, tools []llm.Tool, limit int) ([]llm.Message, bool) {
	if requestFits(messages, tools, limit) {
		return messages, true
	}
	if request.FitMessages == nil || len(messages) < len(request.Messages) {
		return nil, false
	}
	tail := messages[len(request.Messages):]
	budget := limit - llm.ToolsSize(tools) - (llm.MessagesSize(tail) - llm.MessagesSize(nil))
	if budget <= 0 {
		return nil, false
	}
	base, fits := request.FitMessages(budget)
	if !fits {
		return nil, false
	}
	fitted := append(base[:len(base):len(base)], tail...)
	return fitted, requestFits(fitted, tools, limit)
}
