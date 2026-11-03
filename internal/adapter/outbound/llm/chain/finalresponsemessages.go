package chain

import "github.com/it-play/sandrone-code-review-bot/internal/core/llm"

func finalResponseMessages(messages []llm.Message, forceJSON bool) []llm.Message {
	instruction := "더 이상 도구를 호출할 수 없습니다. 파일을 추가로 읽지 말고 지금까지 확인한 내용으로 검토를 완료하세요. "
	if forceJSON {
		instruction += "요청한 JSON 객체만 최종 응답으로 반환하세요."
	} else {
		instruction += "요청한 형식의 최종 응답만 반환하세요."
	}
	return append(messages[:len(messages):len(messages)], llm.Message{Role: llm.RoleUser, Content: instruction})
}
