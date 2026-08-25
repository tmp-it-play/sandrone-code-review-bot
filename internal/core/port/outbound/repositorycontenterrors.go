package outbound

import "errors"

var ErrRepositoryContentNotFound = errors.New("저장소 파일을 찾지 못했습니다")
var ErrRepositoryContentUnavailable = errors.New("저장소 파일 내용을 제공할 수 없습니다")
