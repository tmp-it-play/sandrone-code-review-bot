package setting

import "github.com/it-play/sandrone-code-review-bot/internal/core/review"

type RepoConfig struct {
	Language          string
	Tone              Tone
	Temperature       float64
	MaxOutputTokens   int
	MaxPromptChars    int
	MaxFiles          int
	MaxFileChars      int
	IncludeSources    bool
	MaxSourceChars    int
	MaxExtraReads     int
	Exclude           []string
	Include           []string
	MinSeverity       review.Severity
	MaxInlineComments int
	ThreadReply       bool
	Sandrone          SandroneSetting
}

func DefaultRepoConfig() RepoConfig {
	return RepoConfig{
		Language:          "ko",
		Tone:              ToneProfessional,
		Temperature:       0.2,
		MaxOutputTokens:   16384,
		MaxPromptChars:    140000,
		MaxFiles:          40,
		MaxFileChars:      24000,
		IncludeSources:    true,
		MaxSourceChars:    16000,
		MaxExtraReads:     6,
		Exclude:           DefaultExcludes(),
		Include:           nil,
		MinSeverity:       review.SeverityMinor,
		MaxInlineComments: 25,
		ThreadReply:       true,
		Sandrone: SandroneSetting{
			AutoReview:          true,
			AutoReviewOnPush:    false,
			SummaryPlacement:    SummaryPlacementNewComment,
			Providers:           nil,
			MaxInstructionChars: 20000,
			MaxReviewBatches:    4,
			InstructionFiles:    DefaultInstructionFiles(),
		},
	}
}

func DefaultExcludes() []string {
	return []string{
		"**/node_modules/**",
		"**/dist/**",
		"**/build/**",
		"**/out/**",
		"**/vendor/**",
		"**/coverage/**",
		"**/*.min.js",
		"**/*.map",
		"**/*.snap",
		"**/*.lock",
		"**/*.sum",
		"**/package-lock.json",
		"**/pnpm-lock.yaml",
		"**/yarn.lock",
		"**/*.png",
		"**/*.jpg",
		"**/*.jpeg",
		"**/*.gif",
		"**/*.svg",
		"**/*.ico",
		"**/*.pdf",
		"**/*.woff",
		"**/*.woff2",
	}
}

func DefaultInstructionFiles() []string {
	return []string{
		"AGENTS.md",
		"CLAUDE.md",
		".claude/rules/**/*.md",
		".github/copilot-instructions.md",
		"CONTRIBUTING.md",
	}
}
