package llm

func MaskMessages(messages []Message, mask func(string) string) []Message {
	masked := append([]Message{}, messages...)
	for index := range masked {
		masked[index].Content = mask(masked[index].Content)
		masked[index].ToolCalls = append([]ToolCall{}, masked[index].ToolCalls...)
		for callIndex := range masked[index].ToolCalls {
			masked[index].ToolCalls[callIndex].Arguments = mask(masked[index].ToolCalls[callIndex].Arguments)
		}
	}
	return masked
}
