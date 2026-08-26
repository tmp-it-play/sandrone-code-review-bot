package outbound

import "github.com/it-play/sandrone-code-review-bot/internal/core/review"

type Renderer interface {
	SummaryBody(view review.SummaryView) string
	ProgressBody(message string, marker string) string
	InlineBody(finding review.Finding, attribution review.Attribution, style review.Style) string
	NoticeBody(notice review.Notice) string
	ReplyBody(text string, attribution review.Attribution) string
	Marker() string
	SummaryMarker() string
}
