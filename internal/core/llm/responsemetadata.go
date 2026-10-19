package llm

func MergeResponseMetadata(first, next Response) Response {
	first.Content = ""
	first.ToolCalls = nil
	multiple := first.ModelLabel == "복수 모델" || next.ModelLabel == "복수 모델" || first.Provider == "multiple" || next.Provider == "multiple" || first.Model == "multiple" || next.Model == "multiple"
	if (first.Provider != "" || first.Model != "") && (next.Provider != "" || next.Model != "") && (first.Provider != next.Provider || first.Model != next.Model) {
		multiple = true
	}
	if first.Provider == "" && first.Model == "" {
		first.Provider = next.Provider
		first.Model = next.Model
		first.ModelLabel = next.ModelLabel
	}
	if multiple {
		first.Provider = "multiple"
		first.Model = "multiple"
		first.ModelLabel = "복수 모델"
	}
	first.Usage = normalizedMetadataUsage(first.Usage).Add(normalizedMetadataUsage(next.Usage))
	first.ToolExecutions += next.ToolExecutions
	return first
}

func normalizedMetadataUsage(usage Usage) Usage {
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	return usage
}
