package markdown

import (
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"github.com/it-play/sandrone-code-review-bot/internal/core/setting"
)

func englishReviewProse(tone setting.Tone) reviewProse {
	prose := reviewProse{
		summaryHeading: "Pull Request Summary", noFindings: "No issues were identified within the reviewed scope.",
		omittedFindings: "%d lower-priority findings were omitted due to the output size limit.",
		fileCount:       "Changes across %d files", fileHeader: "File", changesHeader: "Changes",
		fallbackCount: "%d findings without inline placement", suggestion: "Suggestion:",
		otherLocations: "Other locations with the same cause: ", moreLocations: " and %d more", incremental: "Changes since the previous review",
		input: "Input", output: "Output", total: "Total", tokens: "tokens", multipleModels: "Multiple models",
		replyHint:      "Enter `/pr-review-reply` on an inline review comment to check whether the issue has been resolved.",
		repositoryLink: "Sandrone Code Review Bot repository",
		severities:     map[review.Severity]string{review.SeverityCritical: "Critical", review.SeverityMajor: "Major", review.SeverityMinor: "Minor", review.SeverityNit: "Nit"},
	}
	switch tone {
	case setting.ToneIntelligent:
		prose.noFindings = "The reviewed scope contains no substantiated findings."
	case setting.TonePolite:
		prose.noFindings = "I found no issues to flag within the reviewed scope."
		prose.replyHint = "You can enter `/pr-review-reply` on an inline review comment, and I’ll check whether the issue has been resolved."
	case setting.ToneSandrone:
		prose.noFindings = "Nothing to flag in the code reviewed."
		prose.omittedFindings = "The output limit leaves out %d lower-priority findings."
		prose.replyHint = "Enter `/pr-review-reply` on an inline comment and I’ll check whether the defect is resolved."
	}
	return prose
}
