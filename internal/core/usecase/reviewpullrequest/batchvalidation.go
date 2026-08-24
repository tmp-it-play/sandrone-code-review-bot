package reviewpullrequest

import (
	"fmt"
	"strings"

	"github.com/it-play/sandrone-code-review-bot/internal/core/parsing"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

func validateBatchResult(files []pullrequest.ChangedFile, result review.Result, report parsing.Report, requireFileNotes bool) error {
	if report.RawFindings > 0 && report.Dropped == report.RawFindings {
		return fmt.Errorf("리뷰 응답의 모든 지적 형식이 잘못되었습니다")
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
	if requireFileNotes {
		for _, file := range files {
			if _, found := notes[file.Path]; !found {
				return fmt.Errorf("%s 파일 검토 결과가 없습니다", file.Path)
			}
		}
	}
	return nil
}
