package parsing

import (
	"fmt"
	"strings"
)

type ReviewResponseValidator struct {
	RequiredPaths []string
}

func (v ReviewResponseValidator) Validate(content string) error {
	result, report, err := (ResultParser{}).Parse(content)
	if err != nil {
		return err
	}
	if report.RawFindings > 0 && report.Dropped == report.RawFindings {
		return ErrAllFindingsInvalid
	}
	if !report.HasSummary || strings.TrimSpace(result.Summary.Overview) == "" {
		return ErrSummaryMissing
	}
	notes := make(map[string]struct{}, len(result.Summary.Files))
	for _, note := range result.Summary.Files {
		if strings.TrimSpace(note.Path) != "" && strings.TrimSpace(note.Note) != "" {
			notes[note.Path] = struct{}{}
		}
	}
	for _, path := range v.RequiredPaths {
		if _, found := notes[path]; !found {
			return fmt.Errorf("%w: %s", ErrRequiredFileNoteMissing, path)
		}
	}
	return nil
}
