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

var ErrNoProviderAvailable = errors.New("사용 가능한 LLM 프로바이더가 없다")

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

func (c *Chain) Complete(ctx context.Context, request llm.Request, executor outbound.ToolExecutor) (llm.Response, error) {
	var lastErr error
	for _, candidate := range c.providers {
		if !allowed(candidate.Name(), request.Providers) {
			continue
		}
		cooling, err := c.cooldown.Active(ctx, candidate.Name())
		if err != nil {
			c.logger.Warn("쿨다운 상태를 읽지 못했다", "provider", candidate.Name(), "error", err)
		}
		if cooling {
			continue
		}
		response, attemptErr := c.attempt(ctx, candidate, request, executor)
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
					c.logger.Warn("쿨다운을 기록하지 못했다", "provider", candidate.Name(), "error", markErr)
				}
			}
		}
		c.observe(ctx, candidate, outcome, status)
		c.logger.Warn("프로바이더 호출에 실패해 다음으로 넘어간다", "provider", candidate.Name(), "error", attemptErr)
	}
	if lastErr == nil {
		return llm.Response{}, ErrNoProviderAvailable
	}
	return llm.Response{}, fmt.Errorf("모든 프로바이더가 실패했다: %w", lastErr)
}

func (c *Chain) attempt(ctx context.Context, candidate outbound.Provider, request llm.Request, executor outbound.ToolExecutor) (llm.Response, error) {
	messages := append([]llm.Message{}, request.Messages...)
	var tools []llm.Tool
	if executor != nil && candidate.Capability().ToolCalling {
		tools = executor.Definitions()
	}
	for round := 0; round < c.maxToolRounds; round++ {
		attempt := request
		attempt.Messages = messages
		attempt.Tools = tools
		response, err := candidate.Complete(ctx, attempt)
		if err != nil {
			return llm.Response{}, err
		}
		if len(tools) == 0 || !response.NeedsToolExecution() {
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
				result = "도구 실행에 실패했다."
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
	return candidate.Complete(ctx, final)
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
		c.logger.Warn("프로바이더 사용량을 기록하지 못했다", "provider", candidate.Name(), "error", err)
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
