package llm

type FailureKind string

const (
	FailureQuota       FailureKind = "quota"
	FailureRateLimited FailureKind = "rate_limited"
	FailureUnavailable FailureKind = "unavailable"
	FailureInvalid     FailureKind = "invalid"
	FailureAuth        FailureKind = "auth"
)

func (k FailureKind) TriggersCooldown() bool {
	return k == FailureQuota || k == FailureRateLimited || k == FailureAuth
}

func (k FailureKind) IsTransient() bool {
	return k == FailureUnavailable
}
