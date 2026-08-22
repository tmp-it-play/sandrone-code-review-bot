package outbound

import "github.com/it-play/sandrone-code-review-bot/internal/core/review"

type Renderer interface {
	SummaryBody(view review.SummaryView) string
	InlineBody(finding review.Finding) string
	NoticeBody(notice review.Notice) string
	ReplyBody(text string) string
	Marker() string
}
