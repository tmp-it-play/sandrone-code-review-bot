package llm

import "errors"

var ErrCompletionDeadline = errors.New("리뷰 모델 처리 시간 예산을 모두 사용했습니다")
