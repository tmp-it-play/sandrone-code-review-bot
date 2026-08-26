package publication

import "errors"

var ErrLeased = errors.New("게시 작업이 다른 worker에 lease되어 있습니다")
var ErrLeaseLost = errors.New("게시 작업 lease를 잃었습니다")
var ErrSuperseded = errors.New("게시 작업이 더 최신 작업으로 대체되었습니다")
var ErrTargetChanged = errors.New("게시 대상의 base 또는 head가 변경되었습니다")
var ErrInlineReviewRejected = errors.New("인라인 리뷰가 유효성 검사에서 거부되었습니다")
