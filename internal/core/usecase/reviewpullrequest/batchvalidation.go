package reviewpullrequest

import (
	"fmt"
	"strings"

	"github.com/it-play/sandrone-code-review-bot/internal/core/parsing"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

func validateBatchResult(result review.Result, report parsing.Report) error {
	if !report.HasSummary || strings.TrimSpace(result.Summary.Overview) == "" {
		return fmt.Errorf("리뷰 응답에 전체 요약이 없습니다")
	}
	return nil
}
