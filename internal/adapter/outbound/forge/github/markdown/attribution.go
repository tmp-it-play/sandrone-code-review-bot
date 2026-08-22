package markdown

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

func attributionLine(attribution review.Attribution, extra ...string) string {
	parts := make([]string, 0, 3)
	if attribution.Model != "" {
		parts = append(parts, attribution.Model)
	}
	if attribution.HasUsage() {
		parts = append(parts, usageText(attribution))
	}
	parts = append(parts, extra...)
	if len(parts) == 0 {
		return ""
	}
	return fmt.Sprintf("<sub>%s</sub>", strings.Join(parts, " · "))
}

func usageText(attribution review.Attribution) string {
	segments := make([]string, 0, 3)
	if attribution.PromptTokens > 0 {
		segments = append(segments, "입력 "+thousands(attribution.PromptTokens))
	}
	if attribution.CompletionTokens > 0 {
		segments = append(segments, "출력 "+thousands(attribution.CompletionTokens))
	}
	total := attribution.TotalTokens
	if total == 0 {
		total = attribution.PromptTokens + attribution.CompletionTokens
	}
	if total > 0 {
		segments = append(segments, "합계 "+thousands(total)+" 토큰")
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
