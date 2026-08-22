package mapper

import (
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/command"
)

func ToCommandModel(invocation command.Invocation) model.CommandInvocation {
	return model.CommandInvocation{
		Owner:      invocation.Owner,
		Repository: invocation.Repository,
		Number:     invocation.Number,
		Invoker:    invocation.Invoker,
		Kind:       string(invocation.Kind),
		Allowed:    invocation.Allowed,
		Detail:     invocation.Detail,
		OccurredAt: invocation.OccurredAt,
	}
}

func ToCommandInvocation(entry model.CommandInvocation) command.Invocation {
	return command.Invocation{
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
