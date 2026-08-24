package review

import (
	"regexp"
	"strings"
)

var internalCoverageLine = regexp.MustCompile(`(?m)^> 리뷰 커버리지 · 상태[^\r\n]*(?:\r?\n){1,2}`)

var internalUnreviewedSection = regexp.MustCompile(`(?s)<details>\r?\n<summary>이번 리뷰에서 다루지 못한 파일 [0-9]+개</summary>.*?</details>\r?\n?`)

var historicalReviewUnitHeading = regexp.MustCompile(`(?m)^\*\*검토 단위 [1-8]\*\*\r?\n(?:\r?\n)?`)

var internalVerificationNotices = []string{
	"> 독립 검증을 완료하지 못해 diff 근거 검증을 통과한 결과만 게시했습니다. 이 실행은 부분 완료로 기록됩니다.",
	"> 독립 검증을 완료하지 못해 후보 지적을 게시하지 않았습니다. 이 실행은 부분 완료로 기록됩니다.",
}

func SanitizePublicBody(body string) string {
	result := body
	for _, notice := range internalVerificationNotices {
		result = strings.ReplaceAll(result, notice+"\r\n\r\n", "")
		result = strings.ReplaceAll(result, notice+"\n\n", "")
		result = strings.ReplaceAll(result, notice+"\r\n", "")
		result = strings.ReplaceAll(result, notice+"\n", "")
		result = strings.ReplaceAll(result, notice, "")
	}
	result = internalCoverageLine.ReplaceAllString(result, "")
	result = internalUnreviewedSection.ReplaceAllString(result, "")
	return historicalReviewUnitHeading.ReplaceAllString(result, "")
}
