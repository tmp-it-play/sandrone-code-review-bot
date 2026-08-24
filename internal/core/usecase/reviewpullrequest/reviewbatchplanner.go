package reviewpullrequest

import (
	"github.com/it-play/sandrone-code-review-bot/internal/core/batching"
	"github.com/it-play/sandrone-code-review-bot/internal/core/instruction"
	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
	"github.com/it-play/sandrone-code-review-bot/internal/core/prompt"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/setting"
)

type reviewBatchPlanner struct {
	PullRequest      pullrequest.PullRequest
	Inventory        []pullrequest.ChangedFile
	Instructions     instruction.Collection
	Config           setting.RepoConfig
	Incremental      bool
	Extra            string
	ToolsAllowed     bool
	ProviderLimit    int
	MaxReviewBatches int
	Mask             func(string) string
	Paths            *promptPathMap
}

const toolPromptReserve = 3072

const maximumDetailedFileSummaryChanges = 8

func (p reviewBatchPlanner) Build(files []pullrequest.ChangedFile) (batching.Plan, setting.RepoConfig) {
	config, limit := p.promptConfig()
	plan := batching.Plan{}
	current := make([]pullrequest.ChangedFile, 0, len(files))
	for _, file := range (semanticFilePlanner{}).Order(files) {
		candidate := appendFile(current, file)
		if p.fits(candidate, config, limit) {
			current = candidate
			continue
		}
		if len(current) > 0 {
			plan.Batches = append(plan.Batches, current)
			current = nil
		}
		fittedFile, fits := p.fitSingle(file, config, limit)
		if fits {
			current = []pullrequest.ChangedFile{fittedFile}
			continue
		}
		plan.Oversized = append(plan.Oversized, file)
	}
	if len(current) > 0 {
		plan.Batches = append(plan.Batches, current)
	}
	if p.MaxReviewBatches > 0 && len(plan.Batches) > p.MaxReviewBatches {
		for _, batch := range plan.Batches[p.MaxReviewBatches:] {
			plan.Overflow = append(plan.Overflow, batch...)
		}
		plan.Batches = plan.Batches[:p.MaxReviewBatches]
	}
	return plan, config
}

func (p reviewBatchPlanner) Messages(files []pullrequest.ChangedFile, config setting.RepoConfig) []llm.Message {
	return prompt.ReviewPrompt{
		Context: prompt.Context{
			PullRequest:  p.PullRequest,
			Files:        p.maskedFiles(files),
			Inventory:    p.maskedFiles(p.Inventory),
			Instructions: p.Instructions,
			Config:       config,
			Incremental:  p.Incremental,
		},
		Extra:            p.Extra,
		ToolsAllowed:     p.ToolsAllowed,
		IncludeFileNotes: p.includeFileNotes(),
	}.Messages()
}

func (p reviewBatchPlanner) promptConfig() (setting.RepoConfig, int) {
	config := p.Config
	fixed := p.fixedCost(config)
	limit := 0
	if config.MaxPromptChars > 0 {
		limit = fixed + config.MaxPromptChars
	}
	if p.ProviderLimit > 0 && (limit <= 0 || p.ProviderLimit < limit) {
		limit = p.ProviderLimit
	}
	if p.ProviderLimit > 0 {
		contextLimit := p.ProviderLimit - fixed
		if contextLimit < 1 {
			contextLimit = 1
		}
		if config.MaxPromptChars <= 0 || contextLimit < config.MaxPromptChars {
			config.MaxPromptChars = contextLimit
		}
	}
	return config, limit
}

func (p reviewBatchPlanner) fixedCost(config setting.RepoConfig) int {
	context := prompt.Context{Config: config}
	messages := prompt.ReviewPrompt{Context: context, Extra: p.Extra, ToolsAllowed: p.ToolsAllowed, IncludeFileNotes: p.includeFileNotes()}.Messages()
	fixed := messagesSize(messages) - len(context.Render())
	if p.ToolsAllowed {
		fixed += toolPromptReserve
	}
	return fixed
}

func (p reviewBatchPlanner) includeFileNotes() bool {
	return len(p.Inventory) <= maximumDetailedFileSummaryChanges
}

func (p reviewBatchPlanner) fits(files []pullrequest.ChangedFile, config setting.RepoConfig, limit int) bool {
	return limit <= 0 || messagesSize(p.Messages(files, config)) <= limit
}

func (p reviewBatchPlanner) fitSingle(file pullrequest.ChangedFile, config setting.RepoConfig, limit int) (pullrequest.ChangedFile, bool) {
	if p.fits([]pullrequest.ChangedFile{file}, config, limit) {
		return file, true
	}
	if file.Content == "" {
		return file, false
	}
	file.Content = ""
	file.Truncated = true
	return file, p.fits([]pullrequest.ChangedFile{file}, config, limit)
}

func (p reviewBatchPlanner) maskedFiles(files []pullrequest.ChangedFile) []pullrequest.ChangedFile {
	if p.Mask == nil {
		return files
	}
	masked := make([]pullrequest.ChangedFile, len(files))
	for index, file := range files {
		file.Path = p.Paths.PromptPath(file.Path)
		file.PreviousPath = p.Paths.PromptPath(file.PreviousPath)
		file.Status = p.Mask(file.Status)
		file.Patch = p.Mask(file.Patch)
		file.Content = p.Mask(file.Content)
		masked[index] = file
	}
	return masked
}

func appendFile(files []pullrequest.ChangedFile, file pullrequest.ChangedFile) []pullrequest.ChangedFile {
	appended := make([]pullrequest.ChangedFile, len(files)+1)
	copy(appended, files)
	appended[len(files)] = file
	return appended
}

func messagesSize(messages []llm.Message) int {
	return llm.MessagesSize(messages)
}
