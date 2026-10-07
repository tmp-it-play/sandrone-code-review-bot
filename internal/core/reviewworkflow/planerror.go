package reviewworkflow

import "errors"

var ErrPlanChanged = errors.New("저장된 리뷰 계획의 변경 범위가 현재 입력과 다릅니다")
