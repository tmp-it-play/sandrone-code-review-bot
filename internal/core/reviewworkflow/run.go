package reviewworkflow

import (
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

type Run struct {
	ID                 uint64
	Key                string
	InstallationID     int64
	Owner              string
	Repository         string
	Number             int
	BaseSHA            string
	HeadSHA            string
	ConfigHash         string
	PromptVersion      string
	ModelPolicyHash    string
	RequestIdentity    string
	Trigger            review.Trigger
	Status             RunStatus
	SnapshotObservedAt time.Time
	SnapshotOrderKey   string
	TotalCoverage      int
	ReviewedCoverage   int
	FailedCoverage     int
	DeferredCoverage   int
	SkippedCoverage    int
	ExternalCalls      int
	InitialPlanHash    string
	PlanRevision       int
	ErrorSummary       string
	SupersedingHeadSHA string
	StartedAt          time.Time
	HeartbeatAt        time.Time
	TerminalAt         *time.Time
	ExpiresAt          *time.Time
	LeaseToken         string
	LeaseExpiresAt     *time.Time
}
