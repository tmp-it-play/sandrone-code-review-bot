package reviewworkflow

import (
	"strings"
	"time"
)

type TerminalProgressFinalization struct {
	Marker string
}

func NewTerminalProgressFinalization(marker string) *TerminalProgressFinalization {
	if strings.TrimSpace(marker) == "" {
		return nil
	}
	return &TerminalProgressFinalization{Marker: marker}
}

func (f TerminalProgressFinalization) Invalidation(status RunStatus, terminalAt time.Time, expiresAt time.Time) *PublicationInvalidation {
	body := terminalProgressBody(status)
	if strings.TrimSpace(f.Marker) == "" || body == "" {
		return nil
	}
	return &PublicationInvalidation{
		Marker:        f.Marker,
		Reason:        body,
		LastError:     "진행 코멘트 종료 상태 확인 대기 중",
		NextAttemptAt: terminalAt.Add(PublicationInvalidationFenceDelay),
		ExpiresAt:     expiresAt,
	}
}

func terminalProgressBody(status RunStatus) string {
	switch status {
	case RunStatusSkipped:
		return "> [!NOTE]\n> 리뷰할 변경 사항이 없어 이번 리뷰를 종료했습니다."
	case RunStatusSuperseded:
		return "> [!WARNING]\n> 더 최신 변경이 감지되어 이 리뷰를 종료했습니다. 최신 리뷰 실행이 이어서 처리합니다."
	case RunStatusFailed:
		return "> [!CAUTION]\n> 리뷰를 완료하지 못해 이번 실행을 종료했습니다."
	case RunStatusCancelled:
		return "> [!CAUTION]\n> 리뷰 실행이 취소되었습니다."
	default:
		return ""
	}
}
