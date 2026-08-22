package outbound

import (
	"context"

	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
)

type ToolExecutor interface {
	Definitions() []llm.Tool
	Execute(ctx context.Context, call llm.ToolCall) (string, error)
}
