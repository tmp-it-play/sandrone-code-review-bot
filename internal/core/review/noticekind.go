package review

type NoticeKind string

const (
	NoticeRejected    NoticeKind = "rejected"
	NoticeFailed      NoticeKind = "failed"
	NoticeUnavailable NoticeKind = "unavailable"
	NoticeSkipped     NoticeKind = "skipped"
)
