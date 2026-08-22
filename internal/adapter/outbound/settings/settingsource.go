package settings

import (
	"context"

	"github.com/it-play/sandrone-code-review-bot/internal/core/instruction"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/setting"
)

type SettingSource struct {
	loader    *ConfigLoader
	collector *InstructionCollector
}

func NewSettingSource(loader *ConfigLoader, collector *InstructionCollector) *SettingSource {
	return &SettingSource{loader: loader, collector: collector}
}

func (s *SettingSource) RepoConfig(ctx context.Context, target pullrequest.Target) (setting.RepoConfig, error) {
	return s.loader.RepoConfig(ctx, target)
}

func (s *SettingSource) Instructions(ctx context.Context, target pullrequest.Target, config setting.RepoConfig) (instruction.Collection, error) {
	return s.collector.Instructions(ctx, target, config)
}
