package llm

type FailureKind string

const (
	FailureQuota       FailureKind = "quota"
	FailureRateLimited FailureKind = "rate_limited"
	FailureUnavailable FailureKind = "unavailable"
	FailureTimeout     FailureKind = "timeout"
	FailureModel       FailureKind = "model_unavailable"
	FailureAborted     FailureKind = "aborted"
	FailureInvalid     FailureKind = "invalid"
	FailureAuth        FailureKind = "auth"
)

func (k FailureKind) TriggersCooldown() bool {
	return k == FailureQuota || k == FailureRateLimited || k == FailureUnavailable || k == FailureTimeout || k == FailureModel || k == FailureAuth
}

func (k FailureKind) IsTransient() bool {
	return k == FailureUnavailable
}
