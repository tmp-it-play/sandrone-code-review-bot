package markdown

import (
	"fmt"
	"strings"

	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

const fileTableToggleThreshold = 8

type Renderer struct {
	botName string
}

func NewRenderer(botName string) Renderer {
	return Renderer{botName: botName}
}

func (r Renderer) Marker() string {
	return fmt.Sprintf("<!-- %s -->", r.botName)
}

func (r Renderer) closingMarker() string {
	return fmt.Sprintf("<!-- /%s -->", r.botName)
}

func (r Renderer) SummaryBody(view review.SummaryView) string {
	var builder strings.Builder
	prose := reviewProseFor(view.Style)
	builder.WriteString(r.Marker())
	builder.WriteString("\n## Pull Request 요약\n\n")
	if overview := strings.TrimSpace(view.Summary.Overview); overview != "" {
		builder.WriteString(overview)
		builder.WriteString("\n\n")
	}
	if len(view.Summary.Files) > 0 {
		builder.WriteString(r.fileTable(view.Summary.Files))
		builder.WriteString("\n")
	}
	if view.Trigger != review.TriggerCommandSummary && view.InlineCount == 0 && len(view.Fallback) == 0 {
		builder.WriteString(prose.noFindings)
		builder.WriteString("\n\n")
	}
	if len(view.Fallback) > 0 {
		builder.WriteString(r.fallbackSection(view.Fallback, view.Style))
		builder.WriteString("\n")
	}
	if len(view.Unreviewed) > 0 {
		builder.WriteString(r.unreviewedSection(view.Unreviewed, prose))
		builder.WriteString("\n")
	}
	builder.WriteString(r.footer(view))
	builder.WriteString("\n")
	builder.WriteString(r.closingMarker())
	return builder.String()
}

func (r Renderer) InlineBody(finding review.Finding, attribution review.Attribution, style review.Style) string {
	var builder strings.Builder
	builder.WriteString(severityBadge(finding.Severity, style))
	if title := strings.TrimSpace(finding.Title); title != "" {
		builder.WriteString(" ")
		builder.WriteString(title)
	}
	builder.WriteString("\n\n")
	builder.WriteString(strings.TrimSpace(finding.Body))
	if suggestion := normalizeSuggestion(finding.Suggestion); strings.TrimSpace(suggestion) != "" {
		fence := fenceFor(suggestion)
		if finding.SuggestionApplies() {
			builder.WriteString("\n\n")
			builder.WriteString(fence)
			builder.WriteString("suggestion\n")
			builder.WriteString(suggestion)
			builder.WriteString("\n")
			builder.WriteString(fence)
		} else {
			builder.WriteString("\n\n제안:\n\n")
			builder.WriteString(fence)
			builder.WriteString("\n")
			builder.WriteString(suggestion)
			builder.WriteString("\n")
			builder.WriteString(fence)
		}
	}
	if footer := attributionLine(review.Attribution{Model: attribution.Model, Label: attribution.Label}); footer != "" {
		builder.WriteString("\n\n")
		builder.WriteString(footer)
	}
	return builder.String()
}

func (r Renderer) NoticeBody(notice review.Notice) string {
	var builder strings.Builder
	builder.WriteString("> [!")
	builder.WriteString(alertKind(notice.Kind))
	builder.WriteString("]\n")
	for _, line := range strings.Split(strings.TrimSpace(notice.Message), "\n") {
		builder.WriteString("> ")
		builder.WriteString(strings.TrimSpace(line))
		builder.WriteString("\n")
	}
	return strings.TrimRight(builder.String(), "\n")
}

func alertKind(kind review.NoticeKind) string {
	switch kind {
	case review.NoticeFailed, review.NoticeUnavailable:
		return "CAUTION"
	case review.NoticeRetrying, review.NoticeRejected:
		return "WARNING"
	default:
		return "NOTE"
	}
}

func (r Renderer) ReplyBody(text string, attribution review.Attribution) string {
	body := strings.TrimSpace(text)
	footer := attributionLine(attribution)
	if footer == "" {
		return body
	}
	return fmt.Sprintf("%s\n\n%s", body, footer)
}

func (r Renderer) fileTable(notes []review.FileNote) string {
	var builder strings.Builder
	collapse := len(notes) > fileTableToggleThreshold
	if collapse {
		fmt.Fprintf(&builder, "<details>\n<summary>파일별 변경 내용 %d개</summary>\n\n", len(notes))
	}
	builder.WriteString("| 파일 | 변경 내용 |\n| --- | --- |\n")
	for _, note := range notes {
		fmt.Fprintf(&builder, "| `%s` | %s |\n", note.Path, escapeCell(note.Note))
	}
	if collapse {
		builder.WriteString("\n</details>\n")
	}
	return builder.String()
}

func (r Renderer) fallbackSection(findings []review.Finding, style review.Style) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "<details>\n<summary>인라인으로 달지 못한 지적 %d건</summary>\n\n", len(findings))
	for _, finding := range findings {
		location := finding.File
		if finding.Line > 0 {
			location = fmt.Sprintf("%s:%d", finding.File, finding.Line)
		}
		builder.WriteString(severityBadge(finding.Severity, style))
		fmt.Fprintf(&builder, " `%s`", location)
		if title := strings.TrimSpace(finding.Title); title != "" {
			builder.WriteString(" · ")
			builder.WriteString(title)
		}
		builder.WriteString("\n\n")
		builder.WriteString(strings.TrimSpace(finding.Body))
		if suggestion := normalizeSuggestion(finding.Suggestion); strings.TrimSpace(suggestion) != "" {
			fence := fenceFor(suggestion)
			builder.WriteString("\n\n")
			builder.WriteString(fence)
			builder.WriteString("\n")
			builder.WriteString(suggestion)
			builder.WriteString("\n")
			builder.WriteString(fence)
		}
		builder.WriteString("\n\n---\n\n")
	}
	builder.WriteString("</details>\n")
	return builder.String()
}

func (r Renderer) unreviewedSection(files []review.UnreviewedFile, prose reviewProse) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "<details>\n<summary>이번 리뷰에서 다루지 못한 파일 %d개</summary>\n\n", len(files))
	builder.WriteString(prose.unreviewedFiles)
	builder.WriteString("\n\n")
	builder.WriteString("| 파일 | 변경 | 사유 |\n| --- | --- | --- |\n")
	for _, file := range files {
		fmt.Fprintf(&builder, "| `%s` | +%d / -%d | %s |\n", file.Path, file.Additions, file.Deletions, file.Reason)
	}
	builder.WriteString("\n</details>\n")
	return builder.String()
}

func (r Renderer) footer(view review.SummaryView) string {
	if view.Incremental {
		return attributionLine(view.Attribution, "직전 리뷰 이후 변경분")
	}
	return attributionLine(view.Attribution)
}

func escapeCell(text string) string {
	replaced := strings.ReplaceAll(strings.TrimSpace(text), "|", "\\|")
	return strings.ReplaceAll(replaced, "\n", " ")
}
