package markdown

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

func attributionLine(attribution review.Attribution, style review.Style, extra ...string) string {
	parts := make([]string, 0, 3)
	if display := attribution.Display(); display != "" {
		if display == "복수 모델" || attribution.Model == "multiple" || attribution.Provider == "multiple" {
			display = reviewProseFor(style).multipleModels
		}
		parts = append(parts, display)
	}
	if attribution.HasUsage() {
		parts = append(parts, usageText(attribution, style))
	}
	parts = append(parts, extra...)
	if len(parts) == 0 {
		return ""
	}
	return fmt.Sprintf("<sub>%s</sub>", strings.Join(parts, " · "))
}

func usageText(attribution review.Attribution, style review.Style) string {
	segments := make([]string, 0, 3)
	prose := reviewProseFor(style)
	if attribution.PromptTokens > 0 {
		segments = append(segments, prose.input+" "+thousands(attribution.PromptTokens))
	}
	if attribution.CompletionTokens > 0 {
		segments = append(segments, prose.output+" "+thousands(attribution.CompletionTokens))
	}
	total := attribution.TotalTokens
	if total == 0 {
		total = attribution.PromptTokens + attribution.CompletionTokens
	}
	if total > 0 {
		segments = append(segments, prose.total+" "+thousands(total)+" "+prose.tokens)
	}
	return strings.Join(segments, " · ")
}

func thousands(value int) string {
	digits := strconv.Itoa(value)
	if len(digits) <= 3 {
		return digits
	}
	var builder strings.Builder
	lead := len(digits) % 3
	if lead > 0 {
		builder.WriteString(digits[:lead])
	}
	for index := lead; index < len(digits); index += 3 {
		if builder.Len() > 0 {
			builder.WriteByte(',')
		}
		builder.WriteString(digits[index : index+3])
	}
	return builder.String()
}
