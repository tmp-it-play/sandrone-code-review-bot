package progresscomment

import (
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
)

const RefreshInterval = 30 * time.Second
const RefreshLifetime = 4 * time.Hour

type Refresh struct {
	Marker          string
	RunID           uint64
	Target          pullrequest.Target
	MessageTheme    Theme
	Sequence        uint64
	CreatedAt       time.Time
	CreateNotBefore time.Time
	NextRefreshAt   time.Time
	ExpiresAt       time.Time
	LeaseToken      string
	LeaseExpiresAt  time.Time
}
