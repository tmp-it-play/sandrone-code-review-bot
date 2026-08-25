package chain

import (
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
)

type completionFailureCollector struct {
	attempts        []llm.CompletionAttempt
	adaptable       bool
	retryable       bool
	nextAttemptAt   time.Time
	budgetExhausted bool
}

func (c *completionFailureCollector) add(attempt llm.CompletionAttempt) {
	c.attempts = append(c.attempts, attempt)
	c.adaptable = c.adaptable || attempt.Adaptable
	if !attempt.RetryAt.IsZero() {
		c.retryable = true
		if c.nextAttemptAt.IsZero() || attempt.RetryAt.Before(c.nextAttemptAt) {
			c.nextAttemptAt = attempt.RetryAt
		}
	}
	if attempt.Kind == llm.CompletionAttemptBudget {
		c.budgetExhausted = true
	}
}

func (c *completionFailureCollector) failure() *llm.CompletionFailure {
	retryable := c.retryable && !c.budgetExhausted
	nextAttemptAt := c.nextAttemptAt
	if !retryable {
		nextAttemptAt = time.Time{}
	}
	return &llm.CompletionFailure{
		Attempts:        append([]llm.CompletionAttempt(nil), c.attempts...),
		Adaptable:       c.adaptable,
		Retryable:       retryable,
		NextAttemptAt:   nextAttemptAt,
		BudgetExhausted: c.budgetExhausted,
	}
}
