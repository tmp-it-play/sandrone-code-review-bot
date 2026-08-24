package parsing

import (
	"strings"
)

type SummaryResponseValidator struct{}

func (SummaryResponseValidator) Validate(content string) error {
	result, report, err := (ResultParser{}).Parse(content)
	if err != nil {
		return err
	}
	if !report.HasSummary || strings.TrimSpace(result.Summary.Overview) == "" {
		return ErrSummaryMissing
	}
	return nil
}
