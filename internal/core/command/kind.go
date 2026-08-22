package command

type Kind string

const (
	KindUnknown Kind = "unknown"
	KindReview  Kind = "review"
	KindSummary Kind = "summary"
	KindReply   Kind = "reply"
)
