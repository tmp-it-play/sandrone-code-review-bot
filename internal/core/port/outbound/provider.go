package outbound

import (
	"context"

	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
)

type Provider interface {
	Name() string
	Model() string
	Capability() llm.Capability
	Complete(ctx context.Context, request llm.Request) (llm.Response, error)
}
