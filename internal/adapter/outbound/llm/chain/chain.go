package chain

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
	"github.com/it-play/sandrone-code-review-bot/internal/core/port/outbound"
	"github.com/it-play/sandrone-code-review-bot/internal/core/usage"
)

var ErrNoProviderAvailable = errors.New("사용 가능한 LLM 프로바이더가 없습니다")

type Chain struct {
	providers     []outbound.Provider
	cooldown      outbound.Cooldown
	usage         outbound.UsageRepository
	metrics       ProviderMetrics
	clock         outbound.Clock
	logger        *slog.Logger
	cooldownFor   time.Duration
	maxToolRounds int
	providerSlots map[string]chan struct{}
}

func New(providers []outbound.Provider, cooldown outbound.Cooldown, usageRepository outbound.UsageRepository, metrics ProviderMetrics, clock outbound.Clock, logger *slog.Logger, cooldownFor time.Duration) *Chain {
	providerSlots := map[string]chan struct{}{}
	for _, candidate := range providers {
		if limit := candidate.MaxConcurrency(); limit > 0 {
			providerSlots[candidate.Name()] = make(chan struct{}, limit)
		}
	}
	return &Chain{
		providers:     providers,
		cooldown:      cooldown,
		usage:         usageRepository,
		metrics:       metrics,
		clock:         clock,
		logger:        logger,
		cooldownFor:   cooldownFor,
		maxToolRounds: 2,
		providerSlots: providerSlots,
	}
}

func (c *Chain) PromptBudget(providers []string) int {
	for _, candidate := range c.providers {
		if !allowed(candidate.Name(), providers) || candidate.PromptLimit() <= 0 {
			continue
		}
		return candidate.PromptLimit()
	}
	return 0
}

func (c *Chain) PromptBudgetFor(request llm.Request) int {
	if !request.TaskRole.Valid() || !request.DataClassification.Valid() {
		return 0
	}
	for _, candidate := range c.providers {
		if !routeAllowed(candidate, request) || candidate.PromptLimit() <= 0 {
			continue
		}
		return candidate.PromptLimit()
	}
	return 0
}

func (c *Chain) PolicyHashInputs(request llm.Request) llm.PolicyHashInputs {
	identity := llm.PolicyHashInputs{
		Version:               "llm-routing-v3",
		TaskRole:              request.TaskRole,
		DataClassification:    request.DataClassification,
		RequestedProviders:    normalizedNames(request.Providers),
		ExcludedProviders:     normalizedNames(request.ExcludedProviders),
		MaxExternalCalls:      request.ExternalCallBudget.Limit(),
		MaxToolRounds:         c.maxToolRounds,
		MaxTransientRetries:   transientRetries,
		ForceJSON:             request.ForceJSON,
		RequireCompletePrompt: request.RequireCompletePrompt,
		ResponseValidation:    request.ResponseValidation,
	}
	for _, candidate := range c.providers {
		if !routeAllowed(candidate, request) {
			continue
		}
		profile := candidate.Profile()
		identity.EligibleProviders = append(identity.EligibleProviders, llm.ProviderPolicyIdentity{
			Name:               candidate.Name(),
			Model:              candidate.Model(),
			Roles:              append([]llm.TaskRole{}, profile.Roles...),
			PublicDataAllowed:  profile.PublicDataAllowed,
			PrivateCodeAllowed: profile.PrivateCodeAllowed,
			ToolCalling:        candidate.Capability().ToolCalling,
			JSONMode:           candidate.Capability().JSONMode,
			PromptLimit:        candidate.PromptLimit(),
			RequestPolicy:      candidate.RequestPolicy(request),
		})
	}
	return identity
}

func (c *Chain) Complete(ctx context.Context, request llm.Request, executor outbound.ToolExecutor) (llm.Response, error) {
	if !request.TaskRole.Valid() || !request.DataClassification.Valid() {
		return llm.Response{}, ErrRequestPolicyRequired
	}
	if request.ExternalCallBudget == nil {
		return llm.Response{}, ErrExternalCallBudgetRequired
	}
	var lastErr error
	routed := false
	toolExecutions := 0
	usage := llm.Usage{}
	invokedProviders := map[string]struct{}{}
	lastResponse := llm.Response{}
	for _, candidate := range c.providers {
		if !routeAllowed(candidate, request) {
			continue
		}
		routed = true
		var tools []llm.Tool
		if executor != nil && candidate.Capability().ToolCalling {
			tools = executor.Definitions()
		}
		fits := fitsWithinTrimBudget(request.Messages, candidate.PromptLimit()-llm.ToolsSize(tools))
		if request.RequireCompletePrompt {
			fits = requestFits(request.Messages, tools, candidate.PromptLimit())
		}
		if !fits {
			c.logger.Info("입력이 프로바이더 한도에 비해 너무 커서 건너뜁니다",
				"provider", candidate.Name(), "limit", candidate.PromptLimit())
			continue
		}
		release, acquired := c.acquireProvider(candidate)
		if !acquired {
			c.logger.Info("프로바이더 동시 실행 한도에 도달해 건너뜁니다", "provider", candidate.Name())
			continue
		}
		cooling, err := c.cooldown.Active(ctx, candidate.Name())
		if err != nil {
			c.logger.Warn("쿨다운 상태를 읽지 못했습니다", "provider", candidate.Name(), "error", err)
		}
		if cooling {
			release()
			continue
		}
		usedBefore := request.ExternalCallBudget.Used()
		if reserveErr := request.ExternalCallBudget.Reserve(); reserveErr != nil {
			lastResponse.Usage = usage
			lastResponse.ToolExecutions = toolExecutions
			release()
			return lastResponse, reserveErr
		}
		response, observedUsage, attemptErr := c.attemptWithRetry(ctx, candidate, request, executor)
		invoked := request.ExternalCallBudget.Used() > usedBefore
		usage = usage.Add(response.Usage)
		toolExecutions += response.ToolExecutions
		if invoked {
			invokedProviders[candidate.Name()] = struct{}{}
			if response.Provider == "" {
				response.Provider = candidate.Name()
			}
			if response.Model == "" {
				response.Model = candidate.Model()
			}
			lastResponse = response
		}
		if attemptErr == nil && !response.Completed() {
			attemptErr = ErrIncompleteResponse
		}
		if attemptErr == nil && request.ForceJSON && !validJSONObject(response.Content) {
			attemptErr = ErrInvalidJSONResponse
		}
		if attemptErr == nil {
			attemptErr = validateSemanticResponse(request.ResponseValidation, response.Content)
		}
		if attemptErr == nil {
			response.Usage = usage
			response.ToolExecutions = toolExecutions
			if len(invokedProviders) > 1 {
				response.ModelLabel = "복수 모델"
			}
			c.observe(ctx, candidate, response.Model, request.TaskRole, "succeeded", 0, observedUsage)
			release()
			return response, nil
		}
		if errors.Is(attemptErr, llm.ErrExternalCallBudgetExhausted) || errors.Is(attemptErr, llm.ErrExternalCallBudgetUnavailable) {
			lastResponse.Usage = usage
			lastResponse.ToolExecutions = toolExecutions
			if invoked {
				budgetOutcome := "budget_exhausted"
				if errors.Is(attemptErr, llm.ErrExternalCallBudgetUnavailable) {
					budgetOutcome = "budget_unavailable"
				}
				c.observe(ctx, candidate, candidate.Model(), request.TaskRole, budgetOutcome, 0, observedUsage)
			}
			release()
			return lastResponse, attemptErr
		}
		lastErr = attemptErr
		failure, ok := llm.AsFailure(attemptErr)
		outcome := "failed"
		status := 0
		if errors.Is(attemptErr, ErrIncompleteResponse) {
			outcome = "incomplete"
		}
		if errors.Is(attemptErr, ErrInvalidJSONResponse) {
			outcome = "invalid_json"
		}
		if errors.Is(attemptErr, ErrSemanticResponse) {
			outcome = "invalid_semantic_response"
		}
		if ok {
			outcome = string(failure.Kind)
			status = failure.Status
			if failure.Kind.TriggersCooldown() || failure.RetryAfter > 0 {
				c.markProviderCooldown(ctx, candidate, failure.RetryAfter)
			}
		}
		reason := outcome
		if errors.Is(attemptErr, ErrSemanticResponse) {
			reason = semanticFailureReason(attemptErr)
		}
		if invoked {
			c.observe(ctx, candidate, candidate.Model(), request.TaskRole, outcome, status, observedUsage)
		}
		c.logger.Warn("프로바이더 호출에 실패해 다음으로 넘어간다", "provider", candidate.Name(), "outcome", outcome, "status", status, "reason", reason)
		release()
	}
	if !routed {
		return llm.Response{}, ErrNoProviderAllowed
	}
	if lastErr == nil {
		return llm.Response{}, ErrNoProviderAvailable
	}
	lastResponse.Usage = usage
	lastResponse.ToolExecutions = toolExecutions
	if len(invokedProviders) > 1 {
		lastResponse.ModelLabel = "복수 모델"
	}
	return lastResponse, fmt.Errorf("모든 프로바이더가 실패했습니다: %w", lastErr)
}

func (c *Chain) attemptWithRetry(ctx context.Context, candidate outbound.Provider, request llm.Request, executor outbound.ToolExecutor) (llm.Response, llm.Usage, error) {
	var lastErr error
	toolExecutions := 0
	usage := llm.Usage{}
	lastAttemptUsage := llm.Usage{}
	for tryIndex := 0; tryIndex <= transientRetries; tryIndex++ {
		if tryIndex > 0 {
			select {
			case <-ctx.Done():
				return llm.Response{Usage: usage, ToolExecutions: toolExecutions}, llm.Usage{}, ctx.Err()
			case <-time.After(retryPause(tryIndex)):
			}
		}
		response, err := c.attempt(ctx, candidate, request, executor)
		lastAttemptUsage = response.Usage
		usage = usage.Add(response.Usage)
		toolExecutions += response.ToolExecutions
		if err == nil {
			response.Usage = usage
			response.ToolExecutions = toolExecutions
			return response, lastAttemptUsage, nil
		}
		lastErr = err
		failure, ok := llm.AsFailure(err)
		if !ok || !failure.Kind.IsTransient() || failure.RetryAfter > 0 || tryIndex == transientRetries {
			return llm.Response{Usage: usage, ToolExecutions: toolExecutions}, lastAttemptUsage, err
		}
		c.observe(ctx, candidate, candidate.Model(), request.TaskRole, string(failure.Kind), failure.Status, lastAttemptUsage)
		c.logger.Warn("일시적인 오류라 같은 프로바이더로 다시 시도합니다",
			"provider", candidate.Name(), "status", failure.Status, "attempt", tryIndex+1)
	}
	return llm.Response{Usage: usage, ToolExecutions: toolExecutions}, lastAttemptUsage, lastErr
}

func (c *Chain) attempt(ctx context.Context, candidate outbound.Provider, request llm.Request, executor outbound.ToolExecutor) (llm.Response, error) {
	messages := append([]llm.Message{}, request.Messages...)
	var tools []llm.Tool
	if executor != nil && candidate.Capability().ToolCalling {
		tools = executor.Definitions()
	}
	accumulated := llm.Usage{}
	toolExecutions := 0
	models := newCompletionModelTracker()
	for round := 0; round < c.maxToolRounds; round++ {
		if request.RequireCompletePrompt && !requestFits(messages, tools, candidate.PromptLimit()) {
			return llm.Response{Usage: accumulated, ToolExecutions: toolExecutions}, ErrCompletePromptLimitExceeded
		}
		attempt := request
		attempt.Messages = messages
		attempt.Tools = tools
		if !request.RequireCompletePrompt {
			if fitted, trimmed := trimMessages(attempt.Messages, candidate.PromptLimit()-llm.ToolsSize(tools)); trimmed {
				attempt.Messages = fitted
				c.logger.Warn("프로바이더 입력 한도에 맞추어 프롬프트를 줄였습니다", "provider", candidate.Name(), "limit", candidate.PromptLimit())
			}
		}
		if !requestFits(attempt.Messages, tools, candidate.PromptLimit()) {
			return llm.Response{Usage: accumulated, ToolExecutions: toolExecutions}, ErrPromptLimitExceeded
		}
		response, err := c.invoke(ctx, candidate, attempt)
		accumulated = accumulated.Add(response.Usage)
		if err != nil {
			return llm.Response{Usage: accumulated, ToolExecutions: toolExecutions}, err
		}
		response = models.Add(response, candidate.Model())
		if len(tools) == 0 || !response.NeedsToolExecution() {
			response.Usage = accumulated
			response.ToolExecutions += toolExecutions
			return models.Apply(response), nil
		}
		messages = append(messages, llm.Message{
			Role:      llm.RoleAssistant,
			Content:   response.Content,
			ToolCalls: response.ToolCalls,
		})
		for _, call := range response.ToolCalls {
			toolExecutions++
			result, execErr := executor.Execute(ctx, call)
			if execErr != nil {
				result = "도구 실행에 실패했습니다."
			}
			messages = append(messages, llm.Message{
				Role:       llm.RoleTool,
				Content:    result,
				ToolCallID: call.ID,
			})
		}
	}
	final := request
	final.Messages = messages
	final.Tools = nil
	if request.RequireCompletePrompt && !messagesFit(final.Messages, candidate.PromptLimit()) {
		return llm.Response{Usage: accumulated, ToolExecutions: toolExecutions}, ErrCompletePromptLimitExceeded
	} else if !request.RequireCompletePrompt {
		if fitted, trimmed := trimMessages(final.Messages, candidate.PromptLimit()); trimmed {
			final.Messages = fitted
		}
	}
	if !messagesFit(final.Messages, candidate.PromptLimit()) {
		return llm.Response{Usage: accumulated, ToolExecutions: toolExecutions}, ErrPromptLimitExceeded
	}
	response, err := c.invoke(ctx, candidate, final)
	accumulated = accumulated.Add(response.Usage)
	if err != nil {
		return llm.Response{Usage: accumulated, ToolExecutions: toolExecutions}, err
	}
	response = models.Add(response, candidate.Model())
	response.Usage = accumulated
	response.ToolExecutions += toolExecutions
	return models.Apply(response), nil
}

func (c *Chain) invoke(ctx context.Context, candidate outbound.Provider, request llm.Request) (llm.Response, error) {
	return candidate.Complete(ctx, request)
}

func (c *Chain) observe(ctx context.Context, candidate outbound.Provider, model string, role llm.TaskRole, outcome string, status int, tokens llm.Usage) {
	c.metrics.ObserveProvider(candidate.Name(), outcome)
	event := usage.Event{
		Provider:         candidate.Name(),
		Model:            model,
		Role:             string(role),
		Outcome:          outcome,
		Status:           status,
		PromptTokens:     tokens.PromptTokens,
		CompletionTokens: tokens.CompletionTokens,
		TotalTokens:      tokens.TotalTokens,
		OccurredAt:       c.clock.Now(),
	}
	if err := c.usage.Record(ctx, event); err != nil {
		c.logger.Warn("프로바이더 사용량을 기록하지 못했습니다", "provider", candidate.Name(), "error", err)
	}
}

func allowed(name string, permitted []string) bool {
	if len(permitted) == 0 {
		return true
	}
	for _, candidate := range permitted {
		if candidate == name {
			return true
		}
	}
	return false
}

func excluded(name string, exclusions []string) bool {
	for _, candidate := range exclusions {
		if candidate == name {
			return true
		}
	}
	return false
}

func routeAllowed(candidate outbound.Provider, request llm.Request) bool {
	return allowed(candidate.Name(), request.Providers) &&
		!excluded(candidate.Name(), request.ExcludedProviders) &&
		candidate.Profile().Allows(request.TaskRole, request.DataClassification)
}

func normalizedNames(values []string) []string {
	normalized := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	sort.Strings(normalized)
	return normalized
}
