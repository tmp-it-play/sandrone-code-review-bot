package findingverification

import "errors"

var (
	ErrInvalidResponse       = errors.New("검증 모델 응답이 유효하지 않습니다")
	ErrInvalidCandidates     = errors.New("검증 후보가 유효하지 않습니다")
	ErrDuplicateOccurrenceID = errors.New("중복된 occurrence ID입니다")
	ErrUnknownOccurrenceID   = errors.New("알 수 없는 occurrence ID입니다")
	ErrMissingOccurrenceID   = errors.New("누락된 occurrence ID입니다")
	ErrInvalidStatus         = errors.New("알 수 없는 검증 상태입니다")
	ErrMissingReason         = errors.New("검증 이유가 비어 있습니다")
)
