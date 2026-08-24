package review

import (
	"regexp"
	"strings"
)

var internalCoverageLine = regexp.MustCompile(`(?m)^> 리뷰 커버리지 · 상태[^\r\n]*(?:\r?\n){1,2}`)

var internalUnreviewedSection = regexp.MustCompile(`(?s)<details>\r?\n<summary>이번 리뷰에서 다루지 못한 파일 [0-9]+개</summary>.*?</details>\r?\n?`)

var internalReviewUnitHeading = regexp.MustCompile(`(?im)^(?:#{1,6}\s*|\*{1,2})?(?:(?:리뷰|검토)\s*(?:단위|배치)|배치|review\s*(?:unit|batch))\s*#?[0-9]+(?:\s*[/／]\s*[0-9]+)?\s*\*{0,2}\s*[:：.\-–—]*\s*(?:\r?\n){1,2}`)

const internalVerificationNotice = "> 독립 검증을 완료하지 못해 diff 근거 검증을 통과한 결과만 게시했습니다. 이 실행은 부분 완료로 기록됩니다."

func SanitizePublicBody(body string) string {
	result := strings.ReplaceAll(body, internalVerificationNotice+"\r\n\r\n", "")
	result = strings.ReplaceAll(result, internalVerificationNotice+"\n\n", "")
	result = strings.ReplaceAll(result, internalVerificationNotice+"\r\n", "")
	result = strings.ReplaceAll(result, internalVerificationNotice+"\n", "")
	result = strings.ReplaceAll(result, internalVerificationNotice, "")
	result = internalCoverageLine.ReplaceAllString(result, "")
	result = internalUnreviewedSection.ReplaceAllString(result, "")
	return internalReviewUnitHeading.ReplaceAllString(result, "")
}
