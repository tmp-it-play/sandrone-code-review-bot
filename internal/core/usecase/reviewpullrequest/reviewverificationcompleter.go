package reviewpullrequest

import (
	"context"

	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
	"github.com/it-play/sandrone-code-review-bot/internal/core/port/outbound"
)

type reviewVerificationCompleter interface {
	Complete(ctx context.Context, request llm.Request, executor outbound.ToolExecutor) (llm.Response, error)
	PolicyHashInputs(request llm.Request) llm.PolicyHashInputs
}
