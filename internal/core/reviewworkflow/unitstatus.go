package reviewworkflow

type UnitStatus string

const (
	UnitStatusPending   UnitStatus = "pending"
	UnitStatusRunning   UnitStatus = "running"
	UnitStatusSucceeded UnitStatus = "succeeded"
	UnitStatusFailed    UnitStatus = "failed"
	UnitStatusDeferred  UnitStatus = "deferred"
	UnitStatusSplit     UnitStatus = "split"
)
