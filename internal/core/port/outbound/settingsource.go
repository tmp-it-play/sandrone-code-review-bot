package outbound

import (
	"context"

	"github.com/it-play/sandrone-code-review-bot/internal/core/instruction"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/setting"
)

type SettingSource interface {
	RepoConfig(ctx context.Context, target pullrequest.Target) (setting.RepoConfig, error)
	Instructions(ctx context.Context, target pullrequest.Target, config setting.RepoConfig) (instruction.Collection, error)
}
