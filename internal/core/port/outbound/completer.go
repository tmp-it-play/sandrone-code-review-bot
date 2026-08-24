package outbound

import (
	"context"

	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
)

type Completer interface {
	Complete(ctx context.Context, request llm.Request, executor ToolExecutor) (llm.Response, error)
	PromptBudget(providers []string) int
	PromptBudgetFor(request llm.Request) int
	PolicyHashInputs(request llm.Request) llm.PolicyHashInputs
}
