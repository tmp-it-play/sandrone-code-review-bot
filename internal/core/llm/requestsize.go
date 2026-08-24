package llm

import "encoding/json"

const requestEnvelopeReserve = 512
const messageEnvelopeReserve = 32
const toolCallEnvelopeReserve = 48

func MessagesSize(messages []Message) int {
	total := requestEnvelopeReserve
	for _, message := range messages {
		total += len(message.Role) + len(message.Content) + len(message.ToolCallID) + messageEnvelopeReserve
		for _, call := range message.ToolCalls {
			total += len(call.ID) + len(call.Name) + len(call.Arguments) + toolCallEnvelopeReserve
		}
	}
	return total
}

func ToolsSize(tools []Tool) int {
	if len(tools) == 0 {
		return 0
	}
	encoded, err := json.Marshal(tools)
	if err != nil {
		return requestEnvelopeReserve
	}
	return len(encoded) + requestEnvelopeReserve
}

func RequestSize(messages []Message, tools []Tool) int {
	return MessagesSize(messages) + ToolsSize(tools)
}
