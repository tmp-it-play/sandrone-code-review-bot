package llm

import "errors"

var ErrIncompleteResponse = errors.New("LLM 응답이 완료되지 않았습니다")

var ErrInvalidJSONResponse = errors.New("LLM 응답이 유효한 JSON 객체가 아닙니다")

var ErrSemanticResponse = errors.New("LLM 응답이 요청한 결과 계약을 충족하지 않습니다")

var ErrPromptCapacity = errors.New("LLM 입력 용량으로 요청을 구성할 수 없습니다")
