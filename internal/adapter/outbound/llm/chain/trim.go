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
	total := llm.MessagesSize(messages)
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
		target := len(content) - excess
		if target <= 0 {
			trimmed[index].Content = ""
		} else if target <= len(truncationNotice) {
			trimmed[index].Content = prefixBytes(truncationNotice, target)
		} else {
			trimmed[index].Content = prefixBytes(content, target-len(truncationNotice)) + truncationNotice
		}
		excess = llm.MessagesSize(trimmed) - limit
		changed = true
	}
	return trimmed, changed
}

func prefixBytes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(value) <= limit {
		return value
	}
	for limit > 0 && value[limit]&0xc0 == 0x80 {
		limit--
	}
	return value[:limit]
}

const maxTrimRatio = 4

func fitsWithinTrimBudget(messages []llm.Message, limit int) bool {
	if limit <= 0 {
		return true
	}
	total := llm.MessagesSize(messages)
	return total*3 <= limit*maxTrimRatio
}

func messagesFit(messages []llm.Message, limit int) bool {
	if limit <= 0 {
		return true
	}
	return llm.MessagesSize(messages) <= limit
}

func requestFits(messages []llm.Message, tools []llm.Tool, limit int) bool {
	return limit <= 0 || llm.RequestSize(messages, tools) <= limit
}
