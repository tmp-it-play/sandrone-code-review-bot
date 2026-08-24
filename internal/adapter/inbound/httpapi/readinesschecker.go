package httpapi

import "context"

type ReadinessChecker interface {
	Check(context.Context) (map[string]string, bool)
}
