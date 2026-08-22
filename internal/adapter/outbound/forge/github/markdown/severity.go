package markdown

import (
	"fmt"

	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

var severityEmoji = map[review.Severity]string{
	review.SeverityCritical: "🔴",
	review.SeverityMajor:    "🟠",
	review.SeverityMinor:    "🟡",
	review.SeverityNit:      "⚪",
}

func severityBadge(severity review.Severity, style review.Style) string {
	badge := fmt.Sprintf("**`%s`**", severity.Label())
	if !style.Emoji {
		return badge
	}
	mark, ok := severityEmoji[severity]
	if !ok {
		return badge
	}
	return mark + " " + badge
}
