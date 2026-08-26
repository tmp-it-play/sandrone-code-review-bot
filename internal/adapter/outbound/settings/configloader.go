package settings

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/it-play/sandrone-code-review-bot/internal/core/port/outbound"
	"github.com/it-play/sandrone-code-review-bot/internal/core/progresscomment"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"github.com/it-play/sandrone-code-review-bot/internal/core/setting"
	"gopkg.in/yaml.v3"
)

var configCandidates = []string{
	".reviewbot/config.yml",
	".reviewbot/config.yaml",
	".reviewbot.yml",
	".reviewbot.yaml",
}

type ConfigLoader struct {
	content outbound.RepositoryContent
	logger  *slog.Logger
}

func NewConfigLoader(content outbound.RepositoryContent, logger *slog.Logger) *ConfigLoader {
	return &ConfigLoader{content: content, logger: logger}
}

func (l *ConfigLoader) RepoConfig(ctx context.Context, target pullrequest.Target) (setting.RepoConfig, error) {
	config := setting.DefaultRepoConfig()
	for _, ref := range target.PolicyRefs() {
		for _, candidate := range configCandidates {
			body, err := l.content.File(ctx, target, candidate, ref)
			if err != nil {
				if errors.Is(err, outbound.ErrRepositoryContentNotFound) {
					continue
				}
				return config, fmt.Errorf("저장소 리뷰 설정을 읽지 못했습니다: %w", err)
			}
			var raw rawConfig
			if err := yaml.Unmarshal([]byte(body), &raw); err != nil {
				l.logger.Warn("설정 파일을 해석하지 못했습니다", "target", target.FullName(), "path", candidate, "error", err)
				return config, nil
			}
			l.logger.Info("저장소 설정을 읽었습니다", "target", target.FullName(), "path", candidate, "ref", refLabel(ref))
			return boundRepoConfig(merge(config, raw)), nil
		}
	}
	return boundRepoConfig(config), nil
}

func merge(config setting.RepoConfig, raw rawConfig) setting.RepoConfig {
	config = mergeReviewSetting(config, raw.Review)
	if raw.Sandrone != nil {
		config = mergeReviewSetting(config, raw.Sandrone.Review)
		config.Sandrone = mergeSandrone(config.Sandrone, *raw.Sandrone)
	}
	return config
}

func mergeReviewSetting(config setting.RepoConfig, raw rawReviewSetting) setting.RepoConfig {
	if raw.Language != nil {
		config.Language = *raw.Language
	}
	if raw.Tone != nil {
		if tone, ok := setting.ParseTone(*raw.Tone); ok {
			config.Tone = tone
		}
	}
	if raw.AllowStrongTone != nil {
		config.AllowStrongTone = *raw.AllowStrongTone
	}
	if raw.Emoji != nil {
		config.Emoji = *raw.Emoji
	}
	if raw.Temperature != nil {
		config.Temperature = *raw.Temperature
	}
	if raw.MaxOutputTokens != nil {
		config.MaxOutputTokens = *raw.MaxOutputTokens
	}
	if raw.MaxPromptChars != nil {
		config.MaxPromptChars = *raw.MaxPromptChars
	}
	if raw.MaxFiles != nil {
		config.MaxFiles = *raw.MaxFiles
	}
	if raw.MaxFileChars != nil {
		config.MaxFileChars = *raw.MaxFileChars
	}
	if raw.IncludeSources != nil {
		config.IncludeSources = *raw.IncludeSources
	}
	if raw.MaxSourceChars != nil {
		config.MaxSourceChars = *raw.MaxSourceChars
	}
	if raw.MaxExtraReads != nil {
		config.MaxExtraReads = *raw.MaxExtraReads
	}
	if raw.Exclude != nil {
		config.Exclude = append(setting.DefaultExcludes(), (*raw.Exclude)...)
	}
	if raw.Include != nil {
		config.Include = append([]string(nil), (*raw.Include)...)
	}
	if raw.MinSeverity != nil {
		if severity, ok := review.ParseSeverity(*raw.MinSeverity); ok {
			config.MinSeverity = severity
		}
	}
	if raw.MaxInlineComments != nil {
		config.MaxInlineComments = *raw.MaxInlineComments
	}
	if raw.ThreadReply != nil {
		config.ThreadReply = *raw.ThreadReply
	}
	return config
}

func mergeSandrone(current setting.SandroneSetting, raw rawSandrone) setting.SandroneSetting {
	if raw.AutoReview != nil {
		current.AutoReview = *raw.AutoReview
	}
	if raw.AutoReviewOnDraft != nil {
		current.AutoReviewOnDraft = *raw.AutoReviewOnDraft
	}
	if raw.AutoReviewOnPush != nil {
		current.AutoReviewOnPush = *raw.AutoReviewOnPush
	}
	if raw.ProgressMessageTheme != nil {
		if theme, ok := progresscomment.ParseTheme(*raw.ProgressMessageTheme); ok {
			current.ProgressMessageTheme = theme
		}
	}
	if raw.SummaryPlacement != nil {
		if placement, ok := setting.ParseSummaryPlacement(*raw.SummaryPlacement); ok {
			current.SummaryPlacement = placement
		}
	}
	if len(raw.Providers) > 0 {
		current.Providers = raw.Providers
	}
	if raw.MaxInstructionChars != nil {
		current.MaxInstructionChars = *raw.MaxInstructionChars
	}
	if raw.MaxReviewBatches != nil {
		current.MaxReviewBatches = *raw.MaxReviewBatches
	}
	if len(raw.InstructionFiles) > 0 {
		current.InstructionFiles = raw.InstructionFiles
	}
	return current
}

func refLabel(ref string) string {
	if ref == "" {
		return "default"
	}
	return ref
}
