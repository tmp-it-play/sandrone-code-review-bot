package chain

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/observability"
	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
	"github.com/it-play/sandrone-code-review-bot/internal/core/port/outbound"
	"github.com/it-play/sandrone-code-review-bot/internal/core/usage"
)

var ErrNoProviderAvailable = errors.New("사용 가능한 LLM 프로바이더가 없습니다")

type Chain struct {
	providers     []outbound.Provider
	cooldown      outbound.Cooldown
	usage         outbound.UsageRepository
	metrics       *observability.Metrics
	clock         outbound.Clock
	logger        *slog.Logger
	cooldownFor   time.Duration
	maxToolRounds int
}

func New(providers []outbound.Provider, cooldown outbound.Cooldown, usageRepository outbound.UsageRepository, metrics *observability.Metrics, clock outbound.Clock, logger *slog.Logger, cooldownFor time.Duration) *Chain {
	return &Chain{
		providers:     providers,
		cooldown:      cooldown,
		usage:         usageRepository,
		metrics:       metrics,
		clock:         clock,
		logger:        logger,
		cooldownFor:   cooldownFor,
		maxToolRounds: 4,
	}
}

func (c *Chain) PromptBudget() int {
	budget := 0
	for _, candidate := range c.providers {
		limit := candidate.PromptLimit()
		if limit <= 0 {
			continue
		}
		if budget == 0 || limit < budget {
			budget = limit
		}
	}
	return budget
}

func (c *Chain) Complete(ctx context.Context, request llm.Request, executor outbound.ToolExecutor) (llm.Response, error) {
	var lastErr error
	for _, candidate := range c.providers {
		if !allowed(candidate.Name(), request.Providers) {
			continue
		}
		cooling, err := c.cooldown.Active(ctx, candidate.Name())
		if err != nil {
			c.logger.Warn("쿨다운 상태를 읽지 못했습니다", "provider", candidate.Name(), "error", err)
		}
		if cooling {
			continue
		}
		response, attemptErr := c.attemptWithRetry(ctx, candidate, request, executor)
		if attemptErr == nil {
			c.observe(ctx, candidate, "succeeded", 0)
			return response, nil
		}
		lastErr = attemptErr
		failure, ok := llm.AsFailure(attemptErr)
		outcome := "failed"
		status := 0
		if ok {
			outcome = string(failure.Kind)
			status = failure.Status
			if failure.Kind.TriggersCooldown() {
				if markErr := c.cooldown.Mark(ctx, candidate.Name(), c.cooldownFor); markErr != nil {
					c.logger.Warn("쿨다운을 기록하지 못했습니다", "provider", candidate.Name(), "error", markErr)
				}
			}
		}
		c.observe(ctx, candidate, outcome, status)
		c.logger.Warn("프로바이더 호출에 실패해 다음으로 넘어간다", "provider", candidate.Name(), "error", attemptErr)
	}
	if lastErr == nil {
		return llm.Response{}, ErrNoProviderAvailable
	}
	return llm.Response{}, fmt.Errorf("모든 프로바이더가 실패했습니다: %w", lastErr)
}

func (c *Chain) attemptWithRetry(ctx context.Context, candidate outbound.Provider, request llm.Request, executor outbound.ToolExecutor) (llm.Response, error) {
	var lastErr error
	for tryIndex := 0; tryIndex <= transientRetries; tryIndex++ {
		if tryIndex > 0 {
			select {
			case <-ctx.Done():
				return llm.Response{}, ctx.Err()
			case <-time.After(retryPause(tryIndex)):
			}
		}
		response, err := c.attempt(ctx, candidate, request, executor)
		if err == nil {
			return response, nil
		}
		lastErr = err
		failure, ok := llm.AsFailure(err)
		if !ok || !failure.Kind.IsTransient() || tryIndex == transientRetries {
			return llm.Response{}, err
		}
		c.observe(ctx, candidate, string(failure.Kind), failure.Status)
		c.logger.Warn("일시적인 오류라 같은 프로바이더로 다시 시도합니다",
			"provider", candidate.Name(), "status", failure.Status, "attempt", tryIndex+1)
	}
	return llm.Response{}, lastErr
}

func (c *Chain) attempt(ctx context.Context, candidate outbound.Provider, request llm.Request, executor outbound.ToolExecutor) (llm.Response, error) {
	messages := append([]llm.Message{}, request.Messages...)
	var tools []llm.Tool
	if executor != nil && candidate.Capability().ToolCalling {
		tools = executor.Definitions()
	}
	accumulated := llm.Usage{}
	for round := 0; round < c.maxToolRounds; round++ {
		attempt := request
		attempt.Messages = messages
		attempt.Tools = tools
		if fitted, trimmed := trimMessages(attempt.Messages, candidate.PromptLimit()); trimmed {
			attempt.Messages = fitted
			c.logger.Warn("프로바이더 입력 한도에 맞추어 프롬프트를 줄였습니다", "provider", candidate.Name(), "limit", candidate.PromptLimit())
		}
		response, err := candidate.Complete(ctx, attempt)
		if err != nil {
			return llm.Response{}, err
		}
		accumulated = accumulated.Add(response.Usage)
		if len(tools) == 0 || !response.NeedsToolExecution() {
			response.Usage = accumulated
			return response, nil
		}
		messages = append(messages, llm.Message{
			Role:      llm.RoleAssistant,
			Content:   response.Content,
			ToolCalls: response.ToolCalls,
		})
		for _, call := range response.ToolCalls {
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
	if fitted, trimmed := trimMessages(final.Messages, candidate.PromptLimit()); trimmed {
		final.Messages = fitted
	}
	response, err := candidate.Complete(ctx, final)
	if err != nil {
		return llm.Response{}, err
	}
	response.Usage = accumulated.Add(response.Usage)
	return response, nil
}

func (c *Chain) observe(ctx context.Context, candidate outbound.Provider, outcome string, status int) {
	c.metrics.ObserveProvider(candidate.Name(), outcome)
	event := usage.Event{
		Provider:   candidate.Name(),
		Model:      candidate.Model(),
		Outcome:    outcome,
		Status:     status,
		OccurredAt: c.clock.Now(),
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
