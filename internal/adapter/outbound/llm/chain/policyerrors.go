package chain

import "errors"

var ErrRequestPolicyRequired = errors.New("LLM 요청에 유효한 역할과 데이터 분류가 필요합니다")

var ErrExternalCallBudgetRequired = errors.New("LLM 요청에 공유 외부 호출 예산이 필요합니다")

var ErrExternalCallBudgetExhausted = errors.New("공유 외부 호출 예산을 모두 사용했습니다")

var ErrNoProviderAllowed = errors.New("요청 역할과 데이터 분류에 허용된 LLM 프로바이더가 없습니다")

var ErrCompletePromptLimitExceeded = errors.New("도구 결과를 포함한 전체 프롬프트가 프로바이더 입력 한도를 넘었습니다")

var ErrPromptLimitExceeded = errors.New("프롬프트가 프로바이더 입력 한도를 넘었습니다")

var ErrIncompleteResponse = errors.New("LLM 응답이 완료되지 않았습니다")

var ErrInvalidJSONResponse = errors.New("LLM 응답이 유효한 JSON 객체가 아닙니다")

var ErrSemanticResponse = errors.New("LLM 응답이 요청한 결과 계약을 충족하지 않습니다")
