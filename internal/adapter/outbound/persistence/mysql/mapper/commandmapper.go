package mapper

import (
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/command"
)

func ToCommandModel(invocation command.Invocation) model.CommandInvocation {
	var key *string
	if invocation.Key != "" {
		value := invocation.Key
		key = &value
	}
	return model.CommandInvocation{
		InvocationKey: key,
		Owner:         invocation.Owner,
		Repository:    invocation.Repository,
		Number:        invocation.Number,
		Invoker:       invocation.Invoker,
		Kind:          string(invocation.Kind),
		Allowed:       invocation.Allowed,
		Detail:        invocation.Detail,
		OccurredAt:    invocation.OccurredAt,
	}
}

func ToCommandInvocation(entry model.CommandInvocation) command.Invocation {
	key := ""
	if entry.InvocationKey != nil {
		key = *entry.InvocationKey
	}
	return command.Invocation{
		Key:        key,
		Owner:      entry.Owner,
		Repository: entry.Repository,
		Number:     entry.Number,
		Invoker:    entry.Invoker,
		Kind:       command.Kind(entry.Kind),
		Allowed:    entry.Allowed,
		Detail:     entry.Detail,
		OccurredAt: entry.OccurredAt,
	}
}
