package markdown

import (
	"fmt"
	"strings"

	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

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
	builder.WriteString(r.Marker())
	builder.WriteString("\n## Pull Request 요약\n\n")
	if overview := strings.TrimSpace(view.Summary.Overview); overview != "" {
		builder.WriteString(overview)
		builder.WriteString("\n\n")
	}
	if len(view.Summary.Files) > 0 {
		builder.WriteString("| 파일 | 변경 내용 |\n| --- | --- |\n")
		for _, note := range view.Summary.Files {
			builder.WriteString(fmt.Sprintf("| `%s` | %s |\n", note.Path, escapeCell(note.Note)))
		}
		builder.WriteString("\n")
	}
	if line := r.statusLine(view); line != "" {
		builder.WriteString(line)
		builder.WriteString("\n\n")
	}
	if len(view.Fallback) > 0 {
		builder.WriteString(r.fallbackSection(view.Fallback))
		builder.WriteString("\n")
	}
	if len(view.Unreviewed) > 0 {
		builder.WriteString(r.unreviewedSection(view.Unreviewed))
		builder.WriteString("\n")
	}
	builder.WriteString(r.footer(view))
	builder.WriteString("\n")
	builder.WriteString(r.closingMarker())
	return builder.String()
}

func (r Renderer) InlineReviewBody(attribution review.Attribution) string {
	return attributionLine(review.Attribution{Model: attribution.Model})
}

func (r Renderer) InlineBody(finding review.Finding, attribution review.Attribution) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("**%s**", finding.Severity.Label()))
	if title := strings.TrimSpace(finding.Title); title != "" {
		builder.WriteString(" · ")
		builder.WriteString(title)
	}
	builder.WriteString("\n\n")
	builder.WriteString(strings.TrimSpace(finding.Body))
	if suggestion := strings.TrimSpace(finding.Suggestion); suggestion != "" {
		builder.WriteString("\n\n제안:\n\n```\n")
		builder.WriteString(suggestion)
		builder.WriteString("\n```")
	}
	if footer := attributionLine(review.Attribution{Model: attribution.Model}); footer != "" {
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

func (r Renderer) statusLine(view review.SummaryView) string {
	parts := make([]string, 0, 3)
	if view.InlineCount > 0 {
		parts = append(parts, fmt.Sprintf("인라인 지적 %d건", view.InlineCount))
	}
	if len(view.Fallback) > 0 {
		parts = append(parts, fmt.Sprintf("인라인으로 달지 못한 지적 %d건", len(view.Fallback)))
	}
	if view.SkippedDup > 0 {
		parts = append(parts, fmt.Sprintf("이전 리뷰와 겹쳐 생략 %d건", view.SkippedDup))
	}
	if view.BatchCount > 1 {
		parts = append(parts, fmt.Sprintf("%d회에 나눠 검토", view.BatchCount))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " · ")
}

func (r Renderer) fallbackSection(findings []review.Finding) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("<details>\n<summary>인라인으로 달지 못한 지적 %d건</summary>\n\n", len(findings)))
	for _, finding := range findings {
		location := finding.File
		if finding.Line > 0 {
			location = fmt.Sprintf("%s:%d", finding.File, finding.Line)
		}
		builder.WriteString(fmt.Sprintf("**`%s`** · %s", location, finding.Severity.Label()))
		if title := strings.TrimSpace(finding.Title); title != "" {
			builder.WriteString(" · ")
			builder.WriteString(title)
		}
		builder.WriteString("\n\n")
		builder.WriteString(strings.TrimSpace(finding.Body))
		if suggestion := strings.TrimSpace(finding.Suggestion); suggestion != "" {
			builder.WriteString("\n\n```\n")
			builder.WriteString(suggestion)
			builder.WriteString("\n```")
		}
		builder.WriteString("\n\n---\n\n")
	}
	builder.WriteString("</details>\n")
	return builder.String()
}

func (r Renderer) unreviewedSection(files []review.UnreviewedFile) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("<details>\n<summary>이번 리뷰에서 다루지 못한 파일 %d개</summary>\n\n", len(files)))
	builder.WriteString("아래 파일은 분량 제한으로 이번 리뷰에 담지 못했습니다. 확인이 필요하시면 범위를 좁혀 다시 요청해 주세요.\n\n")
	builder.WriteString("| 파일 | 변경 | 사유 |\n| --- | --- | --- |\n")
	for _, file := range files {
		builder.WriteString(fmt.Sprintf("| `%s` | +%d / -%d | %s |\n", file.Path, file.Additions, file.Deletions, file.Reason))
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
