package chain

import (
	"strings"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
)

func responseFailureLogFields(request llm.Request, response llm.Response, policy llm.ProviderRequestPolicyIdentity, elapsed time.Duration) []any {
	format := "text"
	content := strings.TrimSpace(response.Content)
	if content == "" {
		format = "empty"
	} else if strings.HasPrefix(content, "<tool_call>") {
		format = "tool_call_markup"
	} else if strings.HasPrefix(content, "```") {
		format = "markdown_fence"
	}
	return []any{
		"model", response.Model,
		"task_role", string(request.TaskRole),
		"finish_reason", response.FinishReason,
		"response_format", format,
		"content_bytes", len(response.Content),
		"tool_calls", len(response.ToolCalls),
		"tool_executions", response.ToolExecutions,
		"prompt_tokens", response.Usage.PromptTokens,
		"completion_tokens", response.Usage.CompletionTokens,
		"max_output_tokens", policy.MaxOutputTokens,
		"elapsed_ms", elapsed.Milliseconds(),
	}
}
