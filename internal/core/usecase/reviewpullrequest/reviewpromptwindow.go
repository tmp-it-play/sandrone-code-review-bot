package reviewpullrequest

import (
	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/setting"
)

type reviewPromptWindow struct {
	planner reviewBatchPlanner
	config  setting.RepoConfig
	files   []pullrequest.ChangedFile
	mask    func(string) string
}

func (w reviewPromptWindow) plan(limit, output int) (llm.PromptWindow, bool) {
	if len(w.files) == 0 || output < 1024 {
		return llm.PromptWindow{}, false
	}
	w.planner.RoutePolicy = newReviewRoutePolicy([]llm.RouteCapacity{{PromptChars: limit, MaxOutputTokens: output, UsableOutputTokens: output}}, output, 0)
	w.planner.ProviderLimit = limit
	w.planner.Config = w.config
	w.config, limit = w.planner.promptConfig()
	window := llm.PromptWindow{}
	for low, high := 1, len(w.files); low <= high; {
		n := low + (high-low)/2
		part := w
		part.files = w.files[:n]
		if n > 1 && estimatedReviewOutputTokens(part.files, w.planner.includeFileNotes(), w.planner.RoutePolicy.maximumFindings) > w.planner.RoutePolicy.targetOutput {
			high = n - 1
			continue
		}
		messages := part.messages(part.files)
		if limit > 0 && messagesSize(messages) > limit {
			if n > 1 {
				high = n - 1
				continue
			}
			var fits bool
			messages, fits = part.fit(limit)
			if !fits {
				break
			}
		}
		batch := w.planner.RoutePolicy.BatchPolicy(part.files, w.planner.includeFileNotes())
		window = llm.PromptWindow{Messages: messages, Items: n, MaxOutputTokens: batch.MaxOutputTokens, RequiredOutputTokens: batch.RequiredOutputTokens, FitMessages: part.fit}
		low = n + 1
	}
	return window, window.Items > 0
}

func (w reviewPromptWindow) messages(files []pullrequest.ChangedFile) []llm.Message {
	return llm.MaskMessages(w.planner.Messages(files, w.config), w.mask)
}

func (w reviewPromptWindow) fit(limit int) ([]llm.Message, bool) {
	w.planner.ProviderLimit = limit
	w.planner.Config = w.config
	w.config, limit = w.planner.promptConfig()
	messages := w.messages(w.files)
	if limit <= 0 || messagesSize(messages) <= limit {
		return messages, true
	}
	largest := 0
	for _, file := range w.files {
		largest = max(largest, len(file.Content))
	}
	var best []llm.Message
	for low, high := 0, largest; low <= high; {
		capacity := low + (high-low)/2
		files := make([]pullrequest.ChangedFile, len(w.files))
		for i, file := range w.files {
			files[i] = file.WithSourceLimit(capacity)
		}
		messages = w.messages(files)
		if messagesSize(messages) <= limit {
			best = messages
			low = capacity + 1
		} else {
			high = capacity - 1
		}
	}
	return best, best != nil
}
