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
	return resilientPromptBudget(c.RouteCapacitiesFor(request))
}

func (c *Chain) RouteCapacitiesFor(request llm.Request) []llm.RouteCapacity {
	capacities := make([]llm.RouteCapacity, 0, len(c.providers))
	for _, candidate := range c.providers {
		if !routeAllowed(candidate, request) {
			continue
		}
		policy := candidate.RequestPolicy(request)
		capacities = append(capacities, llm.RouteCapacity{
			Provider:           candidate.Name(),
			Model:              candidate.Model(),
			PromptChars:        candidate.PromptLimit(),
			MaxOutputTokens:    policy.MaxOutputTokens,
			UsableOutputTokens: policy.UsableOutputTokens,
		})
	}
	return capacities
}

func (c *Chain) PolicyHashInputs(request llm.Request) llm.PolicyHashInputs {
	identity := llm.PolicyHashInputs{
		Version:               "llm-routing-v6",
		TaskRole:              request.TaskRole,
		DataClassification:    request.DataClassification,
		RequestedProviders:    normalizedNames(request.Providers),
		ExcludedProviders:     normalizedNames(request.ExcludedProviders),
		MaxExternalCalls:      request.ExternalCallBudget.Limit(),
		MaxToolRounds:         c.maxToolRounds,
		MaxTransientRetries:   transientRetries,
		RequiredOutputTokens:  request.RequiredOutputTokens,
		ForceJSON:             request.ForceJSON,
		RequireCompletePrompt: request.RequireCompletePrompt,
		FailFastOnIncomplete:  request.FailFastOnIncomplete,
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

func (c *Chain) Complete(ctx context.Context, request llm.Request, executor outbound.ToolExecutor) (result llm.Response, resultErr error) {
	if !request.TaskRole.Valid() || !request.DataClassification.Valid() {
		return llm.Response{}, ErrRequestPolicyRequired
	}
	if request.ExternalCallBudget == nil {
		return llm.Response{}, ErrExternalCallBudgetRequired
	}
	if !request.Deadline.IsZero() {
		remaining := request.Deadline.Sub(c.clock.Now())
		if remaining <= 0 {
			return llm.Response{}, llm.ErrCompletionDeadline
		}
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeoutCause(ctx, remaining, llm.ErrCompletionDeadline)
		defer cancel()
		defer func() {
			if resultErr != nil && errors.Is(context.Cause(ctx), llm.ErrCompletionDeadline) {
				resultErr = llm.ErrCompletionDeadline
			}
		}()
	}
	collector := &completionFailureCollector{}
	routed := false
	toolExecutions := 0
	usage := llm.Usage{}
	invokedProviders := map[string]struct{}{}
	lastResponse := llm.Response{}
	for _, candidate := range c.providers {
		if err := ctx.Err(); err != nil {
			lastResponse.Usage = usage
			lastResponse.ToolExecutions = toolExecutions
			return lastResponse, err
		}
		if !routeAllowed(candidate, request) {
			continue
		}
		routed = true
		requestPolicy := candidate.RequestPolicy(request)
		outputLimit := requestPolicy.UsableOutputTokens
		if outputLimit <= 0 {
			outputLimit = requestPolicy.MaxOutputTokens
		}
		if request.RequiredOutputTokens > 0 && outputLimit < request.RequiredOutputTokens {
			cause := fmt.Errorf("프로바이더 출력 한도 %d token이 필요한 %d token보다 작습니다", outputLimit, request.RequiredOutputTokens)
			collector.add(llm.CompletionAttempt{Provider: candidate.Name(), Model: candidate.Model(), Kind: llm.CompletionAttemptOutputLimit, Adaptable: true, Cause: cause})
			c.logger.Info("출력 한도가 요청에 부족해 프로바이더를 건너뜁니다", "provider", candidate.Name(), "limit", outputLimit, "required", request.RequiredOutputTokens)
			continue
		}
		var tools []llm.Tool
		if executor != nil && candidate.Capability().ToolCalling && request.ExternalCallBudget.Remaining() >= 2 {
			tools = executor.Definitions()
		}
		fits := fitsWithinTrimBudget(request.Messages, candidate.PromptLimit()-llm.ToolsSize(tools))
		if request.RequireCompletePrompt {
			fits = requestFits(request.Messages, tools, candidate.PromptLimit())
		}
		if !fits {
			cause := fmt.Errorf("프로바이더 입력 한도 %d자를 초과했습니다", candidate.PromptLimit())
			collector.add(llm.CompletionAttempt{Provider: candidate.Name(), Model: candidate.Model(), Kind: llm.CompletionAttemptPromptLimit, Adaptable: true, Cause: cause})
			c.logger.Info("입력이 프로바이더 한도에 비해 너무 커서 건너뜁니다",
				"provider", candidate.Name(), "limit", candidate.PromptLimit())
			continue
		}
		release, acquired := c.acquireProvider(candidate)
		if !acquired {
			collector.add(llm.CompletionAttempt{Provider: candidate.Name(), Model: candidate.Model(), Kind: llm.CompletionAttemptBusy, RetryAt: c.clock.Now().Add(time.Minute), Cause: ErrNoProviderAvailable})
			c.logger.Info("프로바이더 동시 실행 한도에 도달해 건너뜁니다", "provider", candidate.Name())
			continue
		}
		coolingUntil, cooling, err := c.cooldown.EndsAt(ctx, candidate.Name())
		if err != nil {
			if ctx.Err() != nil {
				release()
				lastResponse.Usage = usage
				lastResponse.ToolExecutions = toolExecutions
				return lastResponse, ctx.Err()
			}
			c.logger.Warn("쿨다운 상태를 읽지 못했습니다", "provider", candidate.Name(), "error", err)
		}
		if cooling {
			collector.add(llm.CompletionAttempt{Provider: candidate.Name(), Model: candidate.Model(), Kind: llm.CompletionAttemptCooling, RetryAt: coolingUntil, Cause: ErrNoProviderAvailable})
			release()
			continue
		}
		usedBefore := request.ExternalCallBudget.Used()
		response, observedUsage, observedElapsed, attemptErr := c.attemptWithRetry(ctx, candidate, request, executor)
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
		if attemptErr != nil && ctx.Err() != nil {
			lastResponse.Usage = usage
			lastResponse.ToolExecutions = toolExecutions
			release()
			return lastResponse, ctx.Err()
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
			c.observe(ctx, candidate, response.Model, request.TaskRole, "succeeded", 0, observedUsage, observedElapsed, nil)
			release()
			return response, nil
		}
		if errors.Is(attemptErr, llm.ErrExternalCallBudgetExhausted) || errors.Is(attemptErr, llm.ErrExternalCallBudgetUnavailable) {
			lastResponse.Usage = usage
			lastResponse.ToolExecutions = toolExecutions
			if ctx.Err() != nil {
				release()
				return lastResponse, ctx.Err()
			}
			if invoked {
				budgetOutcome := "budget_exhausted"
				if errors.Is(attemptErr, llm.ErrExternalCallBudgetUnavailable) {
					budgetOutcome = "budget_unavailable"
				}
				c.observe(ctx, candidate, candidate.Model(), request.TaskRole, budgetOutcome, 0, observedUsage, observedElapsed, nil)
			}
			attemptKind := llm.CompletionAttemptBudget
			retryAt := time.Time{}
			if errors.Is(attemptErr, llm.ErrExternalCallBudgetUnavailable) {
				attemptKind = llm.CompletionAttemptBudgetUnavailable
				retryAt = c.clock.Now().Add(time.Minute)
			}
			collector.add(llm.CompletionAttempt{Provider: candidate.Name(), Model: candidate.Model(), Kind: attemptKind, RetryAt: retryAt, Invoked: invoked, Cause: attemptErr})
			release()
			return lastResponse, collector.failure()
		}
		failure, ok := llm.AsFailure(attemptErr)
		outcome := "failed"
		status := 0
		attemptKind := llm.CompletionAttemptProviderFailure
		adaptable := false
		if errors.Is(attemptErr, ErrIncompleteResponse) {
			outcome = "incomplete"
			attemptKind = llm.CompletionAttemptIncomplete
			adaptable = true
		}
		if errors.Is(attemptErr, ErrInvalidJSONResponse) {
			outcome = "invalid_json"
			attemptKind = llm.CompletionAttemptInvalidJSON
			adaptable = true
		}
		if errors.Is(attemptErr, ErrSemanticResponse) {
			outcome = "invalid_semantic_response"
			attemptKind = llm.CompletionAttemptInvalidSemantic
			adaptable = true
		}
		if errors.Is(attemptErr, ErrPromptLimitExceeded) || errors.Is(attemptErr, ErrCompletePromptLimitExceeded) {
			outcome = "prompt_limit"
			attemptKind = llm.CompletionAttemptPromptLimit
			adaptable = true
		}
		retryAt := time.Time{}
		if ok {
			outcome = string(failure.Kind)
			status = failure.Status
			if failure.Kind.TriggersCooldown() || (failure.Kind != llm.FailureAborted && failure.RetryAfter > 0) {
				cooldownEnd := c.markProviderCooldown(ctx, candidate, failure.RetryAfter)
				if failure.Kind == llm.FailureQuota || failure.Kind == llm.FailureRateLimited || failure.Kind == llm.FailureUnavailable || failure.Kind == llm.FailureTimeout {
					retryAt = cooldownEnd
				}
			}
		}
		reason := outcome
		if errors.Is(attemptErr, ErrSemanticResponse) {
			reason = semanticFailureReason(attemptErr)
		}
		if invoked {
			c.observe(ctx, candidate, candidate.Model(), request.TaskRole, outcome, status, observedUsage, observedElapsed, failure)
		}
		collector.add(llm.CompletionAttempt{Provider: candidate.Name(), Model: candidate.Model(), Kind: attemptKind, RetryAt: retryAt, Invoked: invoked, Adaptable: adaptable, Cause: attemptErr})
		logFields := []any{"provider", candidate.Name(), "outcome", outcome, "status", status, "reason", reason}
		logFields = append(logFields, providerFailureLogFields(failure)...)
		c.logger.Warn("프로바이더 호출에 실패해 다음으로 넘어간다", logFields...)
		release()
		if request.FailFastOnIncomplete && errors.Is(attemptErr, ErrIncompleteResponse) {
			lastResponse.Usage = usage
			lastResponse.ToolExecutions = toolExecutions
			return lastResponse, collector.failure()
		}
	}
	if err := ctx.Err(); err != nil {
		lastResponse.Usage = usage
		lastResponse.ToolExecutions = toolExecutions
		return lastResponse, err
	}
	if !routed {
		collector.add(llm.CompletionAttempt{Kind: llm.CompletionAttemptPolicy, Cause: ErrNoProviderAllowed})
		return llm.Response{}, collector.failure()
	}
	if len(collector.attempts) == 0 {
		return llm.Response{}, ErrNoProviderAvailable
	}
	lastResponse.Usage = usage
	lastResponse.ToolExecutions = toolExecutions
	if len(invokedProviders) > 1 {
		lastResponse.ModelLabel = "복수 모델"
	}
	return lastResponse, collector.failure()
}

func (c *Chain) attemptWithRetry(ctx context.Context, candidate outbound.Provider, request llm.Request, executor outbound.ToolExecutor) (llm.Response, llm.Usage, time.Duration, error) {
	var lastErr error
	toolExecutions := 0
	usage := llm.Usage{}
	lastAttemptUsage := llm.Usage{}
	lastAttemptElapsed := time.Duration(0)
	for tryIndex := 0; tryIndex <= transientRetries; tryIndex++ {
		if tryIndex > 0 {
			select {
			case <-ctx.Done():
				return llm.Response{Usage: usage, ToolExecutions: toolExecutions}, llm.Usage{}, 0, ctx.Err()
			case <-time.After(retryPause(tryIndex)):
			}
		}
		attemptStartedAt := time.Now()
		response, err := c.attempt(ctx, candidate, request, executor)
		lastAttemptElapsed = time.Since(attemptStartedAt)
		lastAttemptUsage = response.Usage
		usage = usage.Add(response.Usage)
		toolExecutions += response.ToolExecutions
		if err == nil {
			response.Usage = usage
			response.ToolExecutions = toolExecutions
			return response, lastAttemptUsage, lastAttemptElapsed, nil
		}
		if ctx.Err() != nil {
			return llm.Response{Usage: usage, ToolExecutions: toolExecutions}, lastAttemptUsage, lastAttemptElapsed, ctx.Err()
		}
		lastErr = err
		failure, ok := llm.AsFailure(err)
		if !ok || !failure.Kind.IsTransient() || failure.RetryAfter > 0 || tryIndex == transientRetries || request.ExternalCallBudget.Remaining() == 0 || lastAttemptElapsed >= maximumImmediateRetryElapsed || failure.Elapsed >= maximumImmediateRetryElapsed {
			return llm.Response{Usage: usage, ToolExecutions: toolExecutions}, lastAttemptUsage, lastAttemptElapsed, err
		}
		c.observe(ctx, candidate, candidate.Model(), request.TaskRole, string(failure.Kind), failure.Status, lastAttemptUsage, lastAttemptElapsed, failure)
		logFields := []any{"provider", candidate.Name(), "status", failure.Status, "attempt", tryIndex + 1}
		logFields = append(logFields, providerFailureLogFields(failure)...)
		c.logger.Warn("일시적인 오류라 같은 프로바이더로 다시 시도합니다", logFields...)
	}
	return llm.Response{Usage: usage, ToolExecutions: toolExecutions}, lastAttemptUsage, lastAttemptElapsed, lastErr
}

func (c *Chain) attempt(ctx context.Context, candidate outbound.Provider, request llm.Request, executor outbound.ToolExecutor) (llm.Response, error) {
	messages := append([]llm.Message{}, request.Messages...)
	var tools []llm.Tool
	if executor != nil && candidate.Capability().ToolCalling && request.ExternalCallBudget.Remaining() >= 2 {
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
		if err := request.ExternalCallBudget.Reserve(); err != nil {
			return llm.Response{Usage: accumulated, ToolExecutions: toolExecutions}, err
		}
		response, err := c.invoke(ctx, candidate, attempt)
		accumulated = accumulated.Add(response.Usage)
		if err != nil {
			return llm.Response{Usage: accumulated, ToolExecutions: toolExecutions}, err
		}
		response = models.Add(response, candidate.Model())
		if len(tools) == 0 {
			response.Usage = accumulated
			response.ToolExecutions += toolExecutions
			return models.Apply(response), nil
		}
		if !response.NeedsToolExecution() {
			policy := candidate.RequestPolicy(request)
			validStructuredResponse := !request.ForceJSON || validJSONObject(response.Content)
			if validStructuredResponse {
				validStructuredResponse = validateSemanticResponse(request.ResponseValidation, response.Content) == nil
			}
			if policy.ForceJSONWithTools || validStructuredResponse {
				response.Usage = accumulated
				response.ToolExecutions += toolExecutions
				return models.Apply(response), nil
			}
			messages = append(messages, llm.Message{Role: llm.RoleUser, Content: "도구를 호출하지 말고 요청한 JSON 객체만 최종 응답으로 반환하세요."})
			tools = nil
			break
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
		if request.ExternalCallBudget.Remaining() < 2 {
			tools = nil
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
	if err := request.ExternalCallBudget.Reserve(); err != nil {
		return llm.Response{Usage: accumulated, ToolExecutions: toolExecutions}, err
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

func (c *Chain) observe(ctx context.Context, candidate outbound.Provider, model string, role llm.TaskRole, outcome string, status int, tokens llm.Usage, elapsed time.Duration, failure *llm.Failure) {
	c.metrics.ObserveProvider(candidate.Name(), outcome)
	event := usage.Event{
		Provider:                   candidate.Name(),
		Model:                      model,
		Role:                       string(role),
		Outcome:                    outcome,
		Status:                     status,
		PromptTokens:               tokens.PromptTokens,
		CompletionTokens:           tokens.CompletionTokens,
		TotalTokens:                tokens.TotalTokens,
		RequestElapsedMilliseconds: elapsed.Milliseconds(),
		OccurredAt:                 c.clock.Now(),
	}
	if failure != nil {
		event.ProviderErrorCode = failure.ProviderErrorCode
		if event.RequestElapsedMilliseconds == 0 {
			event.RequestElapsedMilliseconds = failure.Elapsed.Milliseconds()
		}
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
