package review

type NoticeKind string

const (
	NoticeRejected    NoticeKind = "rejected"
	NoticeRetrying    NoticeKind = "retrying"
	NoticeFailed      NoticeKind = "failed"
	NoticeUnavailable NoticeKind = "unavailable"
	NoticeSkipped     NoticeKind = "skipped"
)
