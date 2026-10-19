package markdown

import (
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"github.com/it-play/sandrone-code-review-bot/internal/core/setting"
)

func koreanReviewProse(tone setting.Tone) reviewProse {
	prose := reviewProse{
		summaryHeading: "Pull Request 요약", noFindings: "검토한 범위에서는 별도로 지적할 문제가 없습니다.",
		omittedFindings: "출력 크기 상한으로 우선순위가 낮은 지적 %d건을 생략했습니다.",
		fileCount:       "파일별 변경 내용 %d개", fileHeader: "파일", changesHeader: "변경 내용",
		fallbackCount: "인라인으로 달지 못한 지적 %d건", suggestion: "제안:",
		otherLocations: "같은 원인의 다른 위치: ", moreLocations: " 외 %d곳", incremental: "직전 리뷰 이후 변경분",
		input: "입력", output: "출력", total: "합계", tokens: "토큰", multipleModels: "복수 모델",
		replyHint:      "인라인 리뷰 코멘트에서 `/pr-review-reply`를 입력하면 해당 지적의 해결 여부를 다시 확인하고 답합니다.",
		repositoryLink: "Sandrone Code Review Bot 저장소 바로가기",
		severities:     map[review.Severity]string{review.SeverityCritical: "치명", review.SeverityMajor: "중요", review.SeverityMinor: "경미", review.SeverityNit: "사소"},
	}
	switch tone {
	case setting.ToneIntelligent:
		prose.noFindings = "검토한 범위에서는 지적할 만한 문제가 확인되지 않았습니다."
	case setting.TonePolite:
		prose.noFindings = "검토한 범위에서는 따로 말씀드릴 문제가 없어요."
		prose.omittedFindings = "출력 크기 제한으로 우선순위가 낮은 지적 %d건은 생략했어요."
		prose.replyHint = "인라인 리뷰 코멘트에 `/pr-review-reply`를 입력하면 해당 지적이 해결됐는지 다시 확인해 드려요."
	case setting.ToneSandrone:
		prose.noFindings = "검토한 범위에선 따로 짚을 결함이 없어."
		prose.omittedFindings = "출력 크기 제한으로 우선순위가 낮은 지적 %d건은 생략했어."
		prose.replyHint = "인라인 리뷰 코멘트에 `/pr-review-reply`를 입력하면 해당 지적이 해결됐는지 다시 확인해 줄게."
	}
	return prose
}
