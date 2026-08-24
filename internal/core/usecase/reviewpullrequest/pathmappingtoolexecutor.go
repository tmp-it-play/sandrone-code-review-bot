package reviewpullrequest

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
	"github.com/it-play/sandrone-code-review-bot/internal/core/port/outbound"
)

const toolResultTruncationNotice = "\n[도구 결과 분량 제한으로 이후 내용 생략]"

type pathMappingToolExecutor struct {
	delegate       outbound.ToolExecutor
	paths          *promptPathMap
	mask           func(string) string
	mutex          sync.Mutex
	remainingChars int
}

func (e *pathMappingToolExecutor) Definitions() []llm.Tool {
	return e.delegate.Definitions()
}

func (e *pathMappingToolExecutor) Execute(ctx context.Context, call llm.ToolCall) (string, error) {
	if call.Name == "read_file" {
		var arguments map[string]any
		if json.Unmarshal([]byte(call.Arguments), &arguments) == nil {
			if path, ok := arguments["path"].(string); ok {
				arguments["path"] = e.paths.RawPath(path)
				if encoded, err := json.Marshal(arguments); err == nil {
					call.Arguments = string(encoded)
				}
			}
		}
	}
	result, err := e.delegate.Execute(ctx, call)
	result = e.paths.Sanitize(result, e.mask)
	e.mutex.Lock()
	allowed := e.remainingChars
	if allowed <= 0 {
		result = ""
		e.remainingChars = 0
	} else if len(result) > allowed {
		contentLimit := allowed - len(toolResultTruncationNotice)
		if contentLimit <= 0 {
			result = truncateUTF8(toolResultTruncationNotice, allowed)
		} else {
			result = truncateUTF8(result, contentLimit) + toolResultTruncationNotice
		}
		e.remainingChars = 0
	} else {
		e.remainingChars -= len(result)
	}
	e.mutex.Unlock()
	return result, err
}
