package reviewworkflow

import "errors"

var ErrRunLeased = errors.New("리뷰 실행이 다른 worker에 lease되어 있습니다")
var ErrRunSuperseded = errors.New("더 최신인 리뷰 실행이 있습니다")
var ErrPublicationLeased = errors.New("다른 리뷰 실행이 게시 권한을 lease하고 있습니다")
var ErrProgressCommentLeased = errors.New("진행 코멘트 정리가 다른 worker에 lease되어 있습니다")
var ErrPublicationIncomplete = errors.New("리뷰 게시 receipt가 완료되지 않았습니다")
var ErrPublicationExpired = errors.New("리뷰 게시 payload의 보존 기한이 끝났습니다")
