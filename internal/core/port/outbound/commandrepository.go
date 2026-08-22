package outbound

import (
	"context"

	"github.com/it-play/sandrone-code-review-bot/internal/core/command"
)

type CommandRepository interface {
	Record(ctx context.Context, invocation command.Invocation) error
	Recent(ctx context.Context, limit int) ([]command.Invocation, error)
}
