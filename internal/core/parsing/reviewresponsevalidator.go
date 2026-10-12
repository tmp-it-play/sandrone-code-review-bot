package parsing

import (
	"encoding/json"
	"strings"
)

type ReviewResponseValidator struct{}

func (v ReviewResponseValidator) Validate(content string) error {
	result, report, err := (ResultParser{}).Parse(content)
	if err != nil {
		return err
	}
	if strings.TrimSpace(result.Summary.Overview) == "" {
		return ErrSummaryMissing
	}
	payload, _ := extractObject(content)
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(payload), &fields); err != nil {
		return err
	}
	if findings := strings.TrimSpace(string(fields["findings"])); !strings.HasPrefix(findings, "[") {
		return ErrFindingsMissing
	}
	if report.RawFindings > 0 && report.Dropped == report.RawFindings {
		return ErrAllFindingsInvalid
	}
	return nil
}
