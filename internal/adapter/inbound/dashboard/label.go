package dashboard

import (
	"fmt"
	"strings"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/command"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

var triggerLabels = map[review.Trigger]string{
	review.TriggerPullRequestOpened:      "PR 열림",
	review.TriggerPullRequestDraftOpened: "Draft PR 열림",
	review.TriggerPullRequestPushed:      "푸시",
	review.TriggerCommandReview:          "리뷰 명령",
	review.TriggerCommandSummary:         "요약 명령",
	review.TriggerCommandReply:           "스레드 답글",
	review.TriggerDashboardRerun:         "수동 재실행",
}

var outcomeLabels = map[review.Outcome]string{
	review.OutcomeSucceeded:   "성공",
	review.OutcomePartial:     "부분 완료",
	review.OutcomeSkipped:     "건너뜀",
	review.OutcomeFailed:      "실패",
	review.OutcomeUnavailable: "사용 불가",
	review.OutcomeSuperseded:  "대체됨",
}

var commandLabels = map[command.Kind]string{
	command.KindReview:  "리뷰",
	command.KindSummary: "요약",
	command.KindReply:   "답글",
	command.KindUnknown: "알 수 없음",
}

var placementLabels = map[review.Placement]string{
	review.PlacementInline:   "인라인",
	review.PlacementFallback: "요약 하단",
}

var failureLabels = map[string]string{
	"quota":                     "한도 소진",
	"rate_limited":              "호출 제한",
	"unavailable":               "서비스 불가",
	"timeout":                   "응답 시간 초과",
	"model_unavailable":         "모델 경로 없음",
	"aborted":                   "요청 중단",
	"invalid":                   "요청 오류",
	"auth":                      "인증 오류",
	"failed":                    "알 수 없는 오류",
	"incomplete":                "불완전 응답",
	"invalid_json":              "JSON 형식 오류",
	"invalid_semantic_response": "근거·결과 검증 실패",
	"budget_exhausted":          "앱 내부 호출 예산 소진",
	"budget_unavailable":        "앱 내부 호출 예약 오류",
}

func failureLabel(kind string, status int, providerErrorCode string, requestElapsedMilliseconds int64) string {
	if kind == "" {
		return ""
	}
	label := lookup(failureLabels, kind, kind)
	details := make([]string, 0, 3)
	if status > 0 {
		details = append(details, fmt.Sprintf("HTTP %d", status))
	}
	if providerErrorCode != "" {
		details = append(details, "code "+providerErrorCode)
	}
	if requestElapsedMilliseconds > 0 {
		details = append(details, "요청 "+(time.Duration(requestElapsedMilliseconds)*time.Millisecond).String())
	}
	if len(details) == 0 {
		return label
	}
	return fmt.Sprintf("%s (%s)", label, strings.Join(details, ", "))
}

func triggerLabel(trigger review.Trigger) string {
	return lookup(triggerLabels, trigger, string(trigger))
}

func outcomeLabel(outcome review.Outcome) string {
	return lookup(outcomeLabels, outcome, string(outcome))
}

func commandLabel(kind command.Kind) string {
	return lookup(commandLabels, kind, string(kind))
}

func placementLabel(placement review.Placement) string {
	return lookup(placementLabels, placement, string(placement))
}

func lookup[K comparable](labels map[K]string, key K, fallback string) string {
	if label, ok := labels[key]; ok {
		return label
	}
	if fallback == "" {
		return "-"
	}
	return fallback
}
