package reviewworkflow

import "errors"

var ErrRunLeased = errors.New("리뷰 실행이 다른 worker에 lease되어 있습니다")
var ErrRunSuperseded = errors.New("더 최신인 리뷰 실행이 있습니다")
var ErrPublicationLeased = errors.New("다른 리뷰 실행이 게시 권한을 lease하고 있습니다")
