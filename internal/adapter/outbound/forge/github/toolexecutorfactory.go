package github

import (
	"github.com/it-play/sandrone-code-review-bot/internal/core/port/outbound"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
)

type ToolExecutorFactory struct {
	content  outbound.RepositoryContent
	masker   outbound.Masker
	maxChars int
}

func NewToolExecutorFactory(content outbound.RepositoryContent, masker outbound.Masker, maxChars int) *ToolExecutorFactory {
	return &ToolExecutorFactory{content: content, masker: masker, maxChars: maxChars}
}

func (f *ToolExecutorFactory) ForTarget(target pullrequest.Target, ref string, maxReads int) outbound.ToolExecutor {
	return &ReadFileExecutor{
		content:   f.content,
		masker:    f.masker,
		target:    target,
		ref:       ref,
		maxChars:  f.maxChars,
		remaining: maxReads,
	}
}
