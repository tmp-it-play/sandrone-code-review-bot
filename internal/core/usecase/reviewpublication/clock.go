package reviewpublication

import "time"

type Clock interface {
	Now() time.Time
}
