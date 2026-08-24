package reviewworkflow

import (
	"errors"
	"strings"
)

type ReviewPublicationFinalization struct {
	Status           RunStatus
	Detail           string
	AdvanceWatermark bool
}

func (f ReviewPublicationFinalization) Validate() error {
	if f.Status != RunStatusComplete && f.Status != RunStatusPartial {
		return errors.New("리뷰 게시 완료 상태가 올바르지 않습니다")
	}
	if strings.TrimSpace(f.Detail) == "" {
		return errors.New("리뷰 게시 완료 상세가 비어 있습니다")
	}
	if f.Status != RunStatusComplete && f.AdvanceWatermark {
		return errors.New("부분 리뷰는 watermark를 전진할 수 없습니다")
	}
	return nil
}
