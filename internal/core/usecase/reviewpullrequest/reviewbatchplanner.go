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
	PullRequest       pullrequest.PullRequest
	Inventory         []pullrequest.ChangedFile
	TotalChangedFiles int
	Instructions      instruction.Collection
	Config            setting.RepoConfig
	Incremental       bool
	Extra             string
	ToolsAllowed      bool
	ProviderLimit     int
	MaxReviewBatches  int
	RoutePolicy       reviewRoutePolicy
	Mask              func(string) string
	Paths             *promptPathMap
}

const maximumDetailedFileSummaryChanges = 8

func (p *reviewBatchPlanner) Build(files []pullrequest.ChangedFile) (batching.Plan, setting.RepoConfig) {
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
		if fallbackPolicy, available := p.RoutePolicy.Fallback(); available {
			p.RoutePolicy = fallbackPolicy
			return p.Build(files)
		}
		for _, overflow := range plan.Batches[p.MaxReviewBatches:] {
			plan.Overflow = append(plan.Overflow, overflow...)
		}
		plan.Batches = plan.Batches[:p.MaxReviewBatches]
	}
	if len(plan.Batches) > 1 {
		var pending []pullrequest.ChangedFile
		for _, batch := range plan.Batches {
			pending = append(pending, batch...)
		}
		plan.Batches = [][]pullrequest.ChangedFile{pending}
	}
	return plan, config
}

func (p reviewBatchPlanner) Messages(files []pullrequest.ChangedFile, config setting.RepoConfig) []llm.Message {
	batchPolicy := p.RoutePolicy.BatchPolicy(files, p.includeFileNotes())
	reviewPrompt := prompt.ReviewPrompt{
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
		MaxFindings:      batchPolicy.MaxFindings,
	}
	messages := reviewPrompt.Messages()
	if p.ProviderLimit > 0 && config.MaxPromptChars > 0 {
		if excess := messagesSize(messages) - p.ProviderLimit; excess > 0 {
			reviewPrompt.Context.Config.MaxPromptChars = max(1, config.MaxPromptChars-excess)
			messages = reviewPrompt.Messages()
		}
	}
	return messages
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
	messages := prompt.ReviewPrompt{Context: context, Extra: p.Extra, ToolsAllowed: p.ToolsAllowed, IncludeFileNotes: p.includeFileNotes(), MaxFindings: p.RoutePolicy.maximumFindings}.Messages()
	return messagesSize(messages) - len(context.Render())
}

func (p reviewBatchPlanner) includeFileNotes() bool {
	changedFiles := p.TotalChangedFiles
	if changedFiles <= 0 {
		changedFiles = len(p.Inventory)
	}
	return changedFiles <= maximumDetailedFileSummaryChanges
}

func (p reviewBatchPlanner) fits(files []pullrequest.ChangedFile, config setting.RepoConfig, limit int) bool {
	promptSize := messagesSize(p.Messages(files, config))
	fileNotes := p.includeFileNotes()
	if p.RoutePolicy.ShouldSplit(files, promptSize, fileNotes) {
		return false
	}
	return (limit <= 0 || promptSize <= limit) && p.RoutePolicy.fitsRoute(promptSize, p.RoutePolicy.BatchPolicy(files, fileNotes).RequiredOutputTokens)
}

func (p reviewBatchPlanner) fitSingle(file pullrequest.ChangedFile, config setting.RepoConfig, limit int) (pullrequest.ChangedFile, bool) {
	for {
		if p.fits([]pullrequest.ChangedFile{file}, config, limit) {
			return file, true
		}
		if file.Content == "" {
			return file, false
		}
		file = file.WithSourceLimit(len(file.Content) / 2)
	}
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
