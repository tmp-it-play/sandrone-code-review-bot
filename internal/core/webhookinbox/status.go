package webhookinbox

type Status string

const (
	StatusPending    Status = "pending"
	StatusProcessing Status = "processing"
	StatusCompleted  Status = "completed"
	StatusFailed     Status = "failed"
)

func (s Status) IsTerminal() bool {
	return s == StatusCompleted || s == StatusFailed
}
