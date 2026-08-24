package reviewpullrequest

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/it-play/sandrone-code-review-bot/internal/core/parsing"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

func validateBatchResult(files []pullrequest.ChangedFile, result review.Result, report parsing.Report) error {
	if report.Dropped > 0 {
		return fmt.Errorf("리뷰 응답에서 형식이 잘못된 지적 %d개를 발견했습니다", report.Dropped)
	}
	if !report.HasSummary || strings.TrimSpace(result.Summary.Overview) == "" {
		return fmt.Errorf("리뷰 응답에 전체 요약이 없습니다")
	}
	notes := make(map[string]struct{}, len(result.Summary.Files))
	for _, note := range result.Summary.Files {
		if strings.TrimSpace(note.Path) == "" || strings.TrimSpace(note.Note) == "" {
			continue
		}
		notes[note.Path] = struct{}{}
	}
	for _, file := range files {
		if _, found := notes[file.Path]; !found {
			return fmt.Errorf("%s 파일 검토 결과가 없습니다", file.Path)
		}
	}
	for _, finding := range result.Findings {
		if utf8.RuneCountInString(finding.Suggestion) > 500 {
			return fmt.Errorf("%s 지적의 suggestion이 허용 길이를 넘었습니다", finding.Title)
		}
		if utf8.RuneCountInString(finding.Evidence) > 1600 {
			return fmt.Errorf("%s 지적의 evidence가 허용 길이를 넘었습니다", finding.Title)
		}
		for _, occurrence := range finding.Occurrences {
			if utf8.RuneCountInString(occurrence.Evidence) > 1600 {
				return fmt.Errorf("%s 지적 occurrence의 evidence가 허용 길이를 넘었습니다", finding.Title)
			}
		}
	}
	return nil
}
