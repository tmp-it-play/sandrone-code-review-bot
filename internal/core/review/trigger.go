package review

type Trigger string

const (
	TriggerPullRequestOpened      Trigger = "pull_request_opened"
	TriggerPullRequestDraftOpened Trigger = "pull_request_draft_opened"
	TriggerPullRequestPushed      Trigger = "pull_request_pushed"
	TriggerCommandReview          Trigger = "command_review"
	TriggerCommandSummary         Trigger = "command_summary"
	TriggerCommandReply           Trigger = "command_reply"
	TriggerDashboardRerun         Trigger = "dashboard_rerun"
)

func (t Trigger) IsAutomatic() bool {
	return t == TriggerPullRequestOpened || t == TriggerPullRequestDraftOpened || t == TriggerPullRequestPushed
}
