package parsing

import (
	"strings"
)

type ReviewResponseValidator struct{}

func (v ReviewResponseValidator) Validate(content string) error {
	result, _, err := (ResultParser{}).Parse(content)
	if err != nil {
		return err
	}
	if strings.TrimSpace(result.Summary.Overview) == "" {
		return ErrSummaryMissing
	}
	return nil
}
