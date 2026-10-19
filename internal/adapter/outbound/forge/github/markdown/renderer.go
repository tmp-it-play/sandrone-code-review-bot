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

func (r Renderer) SummaryMarker() string {
	return fmt.Sprintf("<!-- %s-pr-summary -->", r.botName)
}

func (r Renderer) closingMarker() string {
	return fmt.Sprintf("<!-- /%s -->", r.botName)
}

func (r Renderer) SummaryBody(view review.SummaryView) string {
	var builder strings.Builder
	prose := reviewProseFor(view.Style)
	builder.WriteString(r.Marker())
	fmt.Fprintf(&builder, "\n## %s\n\n", prose.summaryHeading)
	if overview := strings.TrimSpace(view.Summary.Overview); overview != "" {
		builder.WriteString(overview)
		builder.WriteString("\n\n")
	}
	if view.OmittedFindings > 0 {
		fmt.Fprintf(&builder, "> "+prose.omittedFindings+"\n\n", view.OmittedFindings)
	}
	if len(view.Summary.Files) > 0 {
		builder.WriteString(r.fileTable(view.Summary.Files, view.Style))
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
	builder.WriteString(r.footer(view))
	builder.WriteString("\n")
	builder.WriteString(r.closingMarker())
	return builder.String()
}

func (r Renderer) ProgressBody(marker string, style review.Style) string {
	var builder strings.Builder
	prose := reviewProseFor(style)
	builder.WriteString("> [!NOTE]\n")
	fmt.Fprintf(&builder, "> %s\n\n", prose.replyHint)
	fmt.Fprintf(&builder, "<sub>[%s](https://github.com/it-play/sandrone-code-review-bot)</sub>\n\n", prose.repositoryLink)
	builder.WriteString(strings.TrimSpace(marker))
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
	appendOccurrenceLocations(&builder, finding, style)
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
			fmt.Fprintf(&builder, "\n\n%s\n\n", reviewProseFor(style).suggestion)
			builder.WriteString(fence)
			builder.WriteString("\n")
			builder.WriteString(suggestion)
			builder.WriteString("\n")
			builder.WriteString(fence)
		}
	}
	if footer := attributionLine(review.Attribution{Model: attribution.Model, Label: attribution.Label}, style); footer != "" {
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
	case review.NoticeRetrying, review.NoticeRejected, review.NoticeSuperseded:
		return "WARNING"
	default:
		return "NOTE"
	}
}

func (r Renderer) ReplyBody(text string, attribution review.Attribution, style review.Style) string {
	body := strings.TrimSpace(text)
	footer := attributionLine(attribution, style)
	if footer == "" {
		return body
	}
	return fmt.Sprintf("%s\n\n%s", body, footer)
}

func (r Renderer) fileTable(notes []review.FileNote, style review.Style) string {
	var builder strings.Builder
	prose := reviewProseFor(style)
	collapse := len(notes) > fileTableToggleThreshold
	if collapse {
		fmt.Fprintf(&builder, "<details>\n<summary>"+prose.fileCount+"</summary>\n\n", len(notes))
	}
	fmt.Fprintf(&builder, "| %s | %s |\n| --- | --- |\n", prose.fileHeader, prose.changesHeader)
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
	fmt.Fprintf(&builder, "<details>\n<summary>"+reviewProseFor(style).fallbackCount+"</summary>\n\n", len(findings))
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
		appendOccurrenceLocations(&builder, finding, style)
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

func appendOccurrenceLocations(builder *strings.Builder, finding review.Finding, style review.Style) {
	locations := make([]string, 0, 5)
	omitted := 0
	for _, occurrence := range finding.Occurrences {
		primary := occurrence.ID != "" && occurrence.ID == finding.OccurrenceID
		if !primary {
			primary = occurrence.File == finding.File && occurrence.Line == finding.Line && occurrence.EndLine == finding.EndLine
		}
		if primary {
			continue
		}
		if len(locations) >= 5 {
			omitted++
			continue
		}
		location := occurrence.File
		if occurrence.Line > 0 {
			location = fmt.Sprintf("%s:%d", occurrence.File, occurrence.Line)
			if occurrence.EndLine > occurrence.Line {
				location = fmt.Sprintf("%s-%d", location, occurrence.EndLine)
			}
		}
		locations = append(locations, "`"+location+"`")
	}
	if len(locations) == 0 {
		return
	}
	prose := reviewProseFor(style)
	builder.WriteString("\n\n" + prose.otherLocations)
	builder.WriteString(strings.Join(locations, ", "))
	if omitted > 0 {
		fmt.Fprintf(builder, prose.moreLocations, omitted)
	}
}

func (r Renderer) footer(view review.SummaryView) string {
	if view.Incremental {
		return attributionLine(view.Attribution, view.Style, reviewProseFor(view.Style).incremental)
	}
	return attributionLine(view.Attribution, view.Style)
}

func escapeCell(text string) string {
	replaced := strings.ReplaceAll(strings.TrimSpace(text), "|", "\\|")
	return strings.ReplaceAll(replaced, "\n", " ")
}
