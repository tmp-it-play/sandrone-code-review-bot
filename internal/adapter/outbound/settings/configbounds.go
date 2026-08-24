package settings

import (
	"math"

	"github.com/it-play/sandrone-code-review-bot/internal/core/setting"
)

func boundRepoConfig(config setting.RepoConfig) setting.RepoConfig {
	defaults := setting.DefaultRepoConfig()
	config.Temperature = boundedFloat(config.Temperature, 0, 1, defaults.Temperature)
	config.MaxOutputTokens = boundedInt(config.MaxOutputTokens, 256, defaults.MaxOutputTokens, defaults.MaxOutputTokens)
	config.MaxPromptChars = boundedInt(config.MaxPromptChars, 1000, defaults.MaxPromptChars, defaults.MaxPromptChars)
	config.MaxFiles = boundedInt(config.MaxFiles, 1, defaults.MaxFiles, defaults.MaxFiles)
	config.MaxFileChars = boundedInt(config.MaxFileChars, 1000, defaults.MaxFileChars, defaults.MaxFileChars)
	config.MaxSourceChars = boundedInt(config.MaxSourceChars, 0, defaults.MaxSourceChars, defaults.MaxSourceChars)
	config.MaxExtraReads = boundedInt(config.MaxExtraReads, 0, defaults.MaxExtraReads, defaults.MaxExtraReads)
	config.MaxInlineComments = boundedInt(config.MaxInlineComments, 0, defaults.MaxInlineComments, defaults.MaxInlineComments)
	config.Sandrone.MaxInstructionChars = boundedInt(config.Sandrone.MaxInstructionChars, 0, defaults.Sandrone.MaxInstructionChars, defaults.Sandrone.MaxInstructionChars)
	config.Sandrone.MaxReviewBatches = boundedInt(config.Sandrone.MaxReviewBatches, 1, defaults.Sandrone.MaxReviewBatches, defaults.Sandrone.MaxReviewBatches)
	return config
}

func boundedInt(value int, minimum int, maximum int, fallback int) int {
	if value < minimum {
		return fallback
	}
	if value > maximum {
		return maximum
	}
	return value
}

func boundedFloat(value float64, minimum float64, maximum float64, fallback float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return fallback
	}
	if value < minimum {
		return fallback
	}
	if value > maximum {
		return maximum
	}
	return value
}
