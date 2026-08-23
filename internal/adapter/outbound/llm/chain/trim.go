package chain

import (
	"sort"

	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
)

const truncationNotice = "\n\n[프로바이더 입력 한도에 맞추어 이후 내용은 생략되었다]\n"

func trimMessages(messages []llm.Message, limit int) ([]llm.Message, bool) {
	if limit <= 0 {
		return messages, false
	}
	total := 0
	for _, message := range messages {
		total += len(message.Content)
	}
	excess := total - limit
	if excess <= 0 {
		return messages, false
	}

	order := make([]int, 0, len(messages))
	for index, message := range messages {
		if message.Role == llm.RoleUser && len(message.Content) > 0 {
			order = append(order, index)
		}
	}
	if len(order) == 0 {
		return messages, false
	}
	sort.SliceStable(order, func(left, right int) bool {
		return len(messages[order[left]].Content) > len(messages[order[right]].Content)
	})

	trimmed := make([]llm.Message, len(messages))
	copy(trimmed, messages)
	changed := false
	for _, index := range order {
		if excess <= 0 {
			break
		}
		content := trimmed[index].Content
		keep := len(content) - excess - len(truncationNotice)
		if keep < 0 {
			keep = 0
		}
		removed := len(content) - keep
		trimmed[index].Content = content[:keep] + truncationNotice
		excess -= removed - len(truncationNotice)
		changed = true
	}
	return trimmed, changed
}

const maxTrimRatio = 4

func fitsWithinTrimBudget(messages []llm.Message, limit int) bool {
	if limit <= 0 {
		return true
	}
	total := 0
	for _, message := range messages {
		total += len(message.Content)
	}
	return total*3 <= limit*maxTrimRatio
}

func messagesFit(messages []llm.Message, limit int) bool {
	if limit <= 0 {
		return true
	}
	total := 0
	for _, message := range messages {
		total += len(message.Content)
	}
	return total <= limit
}
