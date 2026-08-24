package parsing

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type ReviewResponseValidator struct {
	RequiredPaths []string
}

func (v ReviewResponseValidator) Validate(content string) error {
	result, report, err := (ResultParser{}).Parse(content)
	if err != nil {
		return err
	}
	if report.Dropped > 0 {
		return fmt.Errorf("형식이 잘못된 지적 %d개", report.Dropped)
	}
	if !report.HasSummary || strings.TrimSpace(result.Summary.Overview) == "" {
		return fmt.Errorf("전체 요약이 없습니다")
	}
	notes := make(map[string]struct{}, len(result.Summary.Files))
	for _, note := range result.Summary.Files {
		if strings.TrimSpace(note.Path) != "" && strings.TrimSpace(note.Note) != "" {
			notes[note.Path] = struct{}{}
		}
	}
	for _, path := range v.RequiredPaths {
		if _, found := notes[path]; !found {
			return fmt.Errorf("%s 파일 검토 결과가 없습니다", path)
		}
	}
	for _, finding := range result.Findings {
		if utf8.RuneCountInString(finding.Suggestion) > 500 {
			return fmt.Errorf("%s 지적의 suggestion이 허용 길이를 넘었습니다", finding.Title)
		}
		if utf8.RuneCountInString(finding.Evidence) > 1600 {
			return fmt.Errorf("%s 지적의 evidence가 허용 길이를 넘었습니다", finding.Title)
		}
	}
	return nil
}
