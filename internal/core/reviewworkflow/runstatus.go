package reviewworkflow

type RunStatus string

const (
	RunStatusPlanning   RunStatus = "planning"
	RunStatusRunning    RunStatus = "running"
	RunStatusPublishing RunStatus = "publishing"
	RunStatusComplete   RunStatus = "complete"
	RunStatusPartial    RunStatus = "partial"
	RunStatusFailed     RunStatus = "failed"
	RunStatusSuperseded RunStatus = "superseded"
	RunStatusCancelled  RunStatus = "cancelled"
	RunStatusSkipped    RunStatus = "skipped"
)

func (s RunStatus) IsTerminal() bool {
	switch s {
	case RunStatusComplete, RunStatusPartial, RunStatusFailed, RunStatusSuperseded, RunStatusCancelled, RunStatusSkipped:
		return true
	default:
		return false
	}
}
