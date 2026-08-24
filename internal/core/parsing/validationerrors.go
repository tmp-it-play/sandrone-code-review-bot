package parsing

import "errors"

var ErrAllFindingsInvalid = errors.New("모든 지적의 형식이 잘못되었습니다")

var ErrSummaryMissing = errors.New("요약이 없습니다")

var ErrRequiredFileNoteMissing = errors.New("필수 파일 요약이 없습니다")
