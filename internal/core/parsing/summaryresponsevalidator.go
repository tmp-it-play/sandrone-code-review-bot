package parsing

import (
	"fmt"
	"strings"
)

type SummaryResponseValidator struct{}

func (SummaryResponseValidator) Validate(content string) error {
	result, report, err := (ResultParser{}).Parse(content)
	if err != nil {
		return err
	}
	if !report.HasSummary || strings.TrimSpace(result.Summary.Overview) == "" {
		return fmt.Errorf("요약이 없습니다")
	}
	return nil
}
