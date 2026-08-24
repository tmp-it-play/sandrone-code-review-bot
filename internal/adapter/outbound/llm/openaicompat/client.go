package openaicompat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/llm/provider"
	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
)

type Client struct {
	name           string
	model          string
	displayName    string
	baseURL        string
	apiKey         string
	capability     llm.Capability
	profile        llm.ProviderProfile
	requestProfile provider.RequestProfile
	maxPromptChars int
	maxConcurrency int
	extraHeaders   map[string]string
	httpClient     *http.Client
}

func NewClient(name string, model string, displayName string, baseURL string, apiKey string, capability llm.Capability, profile llm.ProviderProfile, requestProfile provider.RequestProfile, maxPromptChars int, maxConcurrency int, extraHeaders map[string]string, timeout time.Duration) *Client {
	return &Client{
		name:           name,
		model:          model,
		displayName:    displayName,
		baseURL:        strings.TrimRight(baseURL, "/"),
		apiKey:         apiKey,
		capability:     capability,
		profile:        profile,
		requestProfile: requestProfile,
		maxPromptChars: maxPromptChars,
		maxConcurrency: maxConcurrency,
		extraHeaders:   extraHeaders,
		httpClient:     &http.Client{Timeout: timeout},
	}
}

func (c *Client) Name() string {
	return c.name
}

func (c *Client) Model() string {
	return c.model
}

func (c *Client) Capability() llm.Capability {
	return c.capability
}

func (c *Client) Profile() llm.ProviderProfile {
	return c.profile
}

func (c *Client) RequestPolicy(request llm.Request) llm.ProviderRequestPolicyIdentity {
	return c.requestProfile.Identity(request)
}

func (c *Client) PromptLimit() int {
	return c.maxPromptChars
}

func (c *Client) MaxConcurrency() int {
	return c.maxConcurrency
}

func (c *Client) Complete(ctx context.Context, request llm.Request) (llm.Response, error) {
	payload := c.chatRequest(request)
	if request.ForceJSON && c.capability.JSONMode && (len(request.Tools) == 0 || c.requestProfile.ForceJSONWithTools) {
		payload.ResponseFormat = &responseFormat{Type: "json_object"}
	}
	if c.capability.ToolCalling {
		payload.Tools = toChatTools(request.Tools)
		if len(payload.Tools) > 0 {
			payload.ParallelToolCalls = c.requestProfile.ParallelToolCalls
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return llm.Response{}, &llm.Failure{Provider: c.name, Kind: llm.FailureInvalid, Cause: err}
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return llm.Response{}, &llm.Failure{Provider: c.name, Kind: llm.FailureInvalid, Cause: err}
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Authorization", "Bearer "+c.apiKey)
	for key, value := range c.extraHeaders {
		httpRequest.Header.Set(key, value)
	}

	httpResponse, err := c.httpClient.Do(httpRequest)
	if err != nil {
		return llm.Response{}, &llm.Failure{Provider: c.name, Kind: llm.FailureUnavailable, Cause: err}
	}
	defer func() {
		_ = httpResponse.Body.Close()
	}()

	raw, err := io.ReadAll(httpResponse.Body)
	if err != nil {
		return llm.Response{}, &llm.Failure{Provider: c.name, Kind: llm.FailureUnavailable, Cause: err}
	}
	if httpResponse.StatusCode >= 400 {
		return llm.Response{}, &llm.Failure{
			Provider:   c.name,
			Kind:       classify(httpResponse.StatusCode, raw),
			Status:     httpResponse.StatusCode,
			RetryAfter: parseRetryAfter(httpResponse.Header.Get("Retry-After"), time.Now()),
			Cause:      fmt.Errorf("%s", describe(raw)),
		}
	}

	var decoded chatResponse
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return llm.Response{}, &llm.Failure{Provider: c.name, Kind: llm.FailureInvalid, Status: httpResponse.StatusCode, Cause: err}
	}
	if len(decoded.Choices) == 0 {
		return llm.Response{}, &llm.Failure{Provider: c.name, Kind: llm.FailureInvalid, Status: httpResponse.StatusCode, Cause: fmt.Errorf("응답에 선택지가 없습니다")}
	}
	choice := decoded.Choices[0]
	content := choice.Message.Content
	if c.requestProfile.StripLeadingEmptyThought {
		content = strings.TrimPrefix(content, "<|channel>thought\n<channel|>")
	}
	resolvedModel := strings.TrimSpace(decoded.Model)
	if resolvedModel == "" {
		resolvedModel = c.model
	}
	modelLabel := c.displayName
	if resolvedModel != c.model {
		if modelLabel == "" {
			modelLabel = resolvedModel
		} else {
			modelLabel += " [" + resolvedModel + "]"
		}
	}
	return llm.Response{
		Content:      content,
		ToolCalls:    fromChatToolCalls(choice.Message.ToolCalls),
		Provider:     c.name,
		Model:        resolvedModel,
		ModelLabel:   modelLabel,
		FinishReason: choice.FinishReason,
		Usage: llm.Usage{
			PromptTokens:     decoded.Usage.PromptTokens,
			CompletionTokens: decoded.Usage.CompletionTokens,
			TotalTokens:      decoded.Usage.TotalTokens,
		},
	}, nil
}

func (c *Client) chatRequest(request llm.Request) chatRequest {
	payload := chatRequest{
		Model:           c.model,
		Messages:        toChatMessages(request.Messages),
		Temperature:     c.requestProfile.Temperature,
		TopP:            c.requestProfile.TopP,
		ReasoningEffort: c.requestProfile.ReasoningEffort,
	}
	if c.requestProfile.UseRequestTemperature && request.Temperature != 0 {
		temperature := request.Temperature
		payload.Temperature = &temperature
	}
	maxOutputTokens := request.MaxOutputTokens
	if limit := c.requestProfile.OutputTokenLimit; limit > 0 && maxOutputTokens > limit {
		maxOutputTokens = limit
	}
	if c.requestProfile.UseMaxCompletionTokens {
		payload.MaxCompletionTokens = maxOutputTokens
	} else {
		payload.MaxTokens = maxOutputTokens
	}
	if effort := c.requestProfile.NestedReasoningEffort; effort != "" {
		payload.Reasoning = &reasoningConfig{Effort: effort, Exclude: c.requestProfile.ExcludeReasoning}
	}
	if enabled := c.requestProfile.ThinkingEnabled; enabled != nil {
		payload.ChatTemplateKwargs = &chatTemplateKwargs{EnableThinking: *enabled}
	}
	return payload
}

func toChatMessages(messages []llm.Message) []chatMessage {
	converted := make([]chatMessage, 0, len(messages))
	for _, message := range messages {
		entry := chatMessage{
			Role:       string(message.Role),
			Content:    message.Content,
			ToolCallID: message.ToolCallID,
		}
		for _, call := range message.ToolCalls {
			convertedCall := chatToolCall{
				ID:       call.ID,
				Type:     "function",
				Function: chatFunctionCall{Name: call.Name, Arguments: call.Arguments},
			}
			if call.ThoughtSignature != "" {
				convertedCall.ExtraContent = &chatToolCallExtraContent{
					Google: &chatToolCallGoogle{ThoughtSignature: call.ThoughtSignature},
				}
			}
			entry.ToolCalls = append(entry.ToolCalls, convertedCall)
		}
		converted = append(converted, entry)
	}
	return converted
}

func toChatTools(tools []llm.Tool) []chatTool {
	if len(tools) == 0 {
		return nil
	}
	converted := make([]chatTool, 0, len(tools))
	for _, tool := range tools {
		converted = append(converted, chatTool{
			Type: "function",
			Function: chatFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.Parameters,
			},
		})
	}
	return converted
}

func fromChatToolCalls(calls []chatToolCall) []llm.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	converted := make([]llm.ToolCall, 0, len(calls))
	for _, call := range calls {
		thoughtSignature := ""
		if call.ExtraContent != nil && call.ExtraContent.Google != nil {
			thoughtSignature = call.ExtraContent.Google.ThoughtSignature
		}
		converted = append(converted, llm.ToolCall{
			ID:               call.ID,
			Name:             call.Function.Name,
			Arguments:        call.Function.Arguments,
			ThoughtSignature: thoughtSignature,
		})
	}
	return converted
}

func classify(status int, raw []byte) llm.FailureKind {
	text := strings.ToLower(describe(raw))
	switch {
	case status == 429:
		if strings.Contains(text, "quota") || strings.Contains(text, "exceeded your current") {
			return llm.FailureQuota
		}
		return llm.FailureRateLimited
	case status == 401 || status == 403:
		return llm.FailureAuth
	case status == 402:
		return llm.FailureQuota
	case status >= 500:
		return llm.FailureUnavailable
	default:
		return llm.FailureInvalid
	}
}

func describe(raw []byte) string {
	var decoded apiError
	if err := json.Unmarshal(raw, &decoded); err == nil {
		if text := decoded.text(); text != "" {
			return text
		}
	}
	if len(raw) > 400 {
		return string(raw[:400])
	}
	return string(raw)
}
