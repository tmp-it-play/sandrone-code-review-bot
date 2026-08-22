package reviewpullrequest

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/batching"
	"github.com/it-play/sandrone-code-review-bot/internal/core/dedupe"
	"github.com/it-play/sandrone-code-review-bot/internal/core/job"
	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
	"github.com/it-play/sandrone-code-review-bot/internal/core/mapping"
	"github.com/it-play/sandrone-code-review-bot/internal/core/port/outbound"
	"github.com/it-play/sandrone-code-review-bot/internal/core/prompt"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"github.com/it-play/sandrone-code-review-bot/internal/core/selection"
	"github.com/it-play/sandrone-code-review-bot/internal/core/setting"
)

type UseCase struct {
	deps Dependencies
}

func New(deps Dependencies) *UseCase {
	return &UseCase{deps: deps}
}

func (u *UseCase) Execute(ctx context.Context, task job.ReviewJob) error {
	startedAt := u.deps.Clock.Now()
	config, err := u.deps.Settings.RepoConfig(ctx, task.Target)
	if err != nil {
		return u.fail(ctx, task, startedAt, "설정을 읽지 못했습니다", err)
	}
	if reason, skipped := skipReason(task, config); skipped {
		u.save(ctx, task, startedAt, review.OutcomeSkipped, reason, llm.Response{}, 0, 0)
		return nil
	}

	target := task.Target
	request, err := u.deps.Source.PullRequest(ctx, target)
	if err != nil {
		return u.fail(ctx, task, startedAt, "Pull Request를 읽지 못했습니다", err)
	}
	if target.HeadSHA == "" {
		target.HeadSHA = request.HeadSHA
	}

	files, incremental, err := u.collectFiles(ctx, target, request, task)
	if err != nil {
		return u.fail(ctx, task, startedAt, "변경 파일을 읽지 못했습니다", err)
	}
	chosen := selection.FileSelector{
		Include:  config.Include,
		Exclude:  config.Exclude,
		MaxFiles: config.MaxFiles,
	}.Select(files)
	if chosen.IsEmpty() {
		u.notify(ctx, target, task, review.Notice{Kind: review.NoticeSkipped, Message: "리뷰할 변경 사항이 없습니다."})
		u.save(ctx, task, startedAt, review.OutcomeSkipped, "리뷰 대상 파일 없음", llm.Response{}, 0, 0)
		u.rememberHead(ctx, target)
		return nil
	}
	loaded := u.loadSources(ctx, target, chosen.Files, config)

	instructions, err := u.deps.Settings.Instructions(ctx, target, config)
	if err != nil {
		u.deps.Logger.Warn("지침 문서를 읽지 못했습니다", "target", target.Reference(), "error", err)
	}

	inventory := inventoryOf(loaded, chosen.Skipped)
	reserved := instructions.TotalSize() + len(request.Body) + inventoryCost(inventory)
	plan := batching.Batcher{
		MaxChars:   batchBudget(config.MaxPromptChars, u.deps.Completer.PromptBudget()),
		MaxBatches: config.Sandrone.MaxReviewBatches,
	}.Split(loaded, reserved)

	executor := u.executor(target, config)
	gathered := review.Result{}
	usage := llm.Usage{}
	response := llm.Response{}
	reviewed := make([]pullrequest.ChangedFile, 0, len(loaded))
	failed := make([]pullrequest.ChangedFile, 0)

	for index, batch := range plan.Batches {
		messages := prompt.ReviewPrompt{
			Context: prompt.Context{
				PullRequest:  request,
				Files:        batch,
				Inventory:    inventory,
				Instructions: instructions,
				Config:       config,
				Incremental:  incremental,
			},
			Extra:        task.Instruction,
			ToolsAllowed: executor != nil,
		}.Messages()

		batchResponse, batchErr := u.deps.Completer.Complete(ctx, llm.Request{
			Messages:        messages,
			Temperature:     config.Temperature,
			MaxOutputTokens: config.MaxOutputTokens,
			Providers:       config.Sandrone.Providers,
			ForceJSON:       true,
		}, executor)
		if batchErr != nil {
			if index == 0 {
				return u.fail(ctx, task, startedAt, "리뷰 모델을 호출하지 못했습니다", batchErr)
			}
			u.deps.Logger.Warn("일부 배치를 리뷰하지 못했습니다", "target", target.Reference(), "batch", index+1, "error", batchErr)
			failed = append(failed, batch...)
			continue
		}
		usage = usage.Add(batchResponse.Usage)
		response = batchResponse

		batchResult, parseErr := u.deps.Parser.Parse(batchResponse.Content)
		if parseErr != nil {
			if index == 0 {
				return u.fail(ctx, task, startedAt, "모델 응답을 해석하지 못했습니다", parseErr)
			}
			u.deps.Logger.Warn("일부 배치의 응답을 해석하지 못했습니다", "target", target.Reference(), "batch", index+1, "error", parseErr)
			failed = append(failed, batch...)
			continue
		}
		if index == 0 {
			gathered.Summary.Overview = batchResult.Summary.Overview
		}
		gathered.Summary.Files = append(gathered.Summary.Files, batchResult.Summary.Files...)
		gathered.Findings = append(gathered.Findings, batchResult.Findings...)
		reviewed = append(reviewed, batch...)
	}

	findings := selection.SeverityFilter{Minimum: config.MinSeverity}.Apply(gathered.Findings)
	known, err := u.deps.Findings.Fingerprints(ctx, target)
	if err != nil {
		u.deps.Logger.Warn("기존 지적을 읽지 못했습니다", "target", target.Reference(), "error", err)
		known = map[string]struct{}{}
	}
	findings, duplicates := dedupe.DuplicateFilter{Known: known}.Apply(findings)
	findings = mapping.PositionMapper{MaxInline: config.MaxInlineComments}.Map(findings, reviewed)

	placed := review.Result{Summary: gathered.Summary, Findings: findings}
	response.Usage = usage
	attribution := attributionOf(response)
	view := review.SummaryView{
		Summary:     placed.Summary,
		Fallback:    placed.Fallback(),
		InlineCount: len(placed.Inline()),
		Attribution: attribution,
		Trigger:     task.Trigger,
		Incremental: incremental,
		SkippedDup:  duplicates,
		Unreviewed:  unreviewedOf(chosen.Skipped, plan.Overflow, failed),
	}

	if err := u.publish(ctx, target, view, placed, attribution); err != nil {
		return u.fail(ctx, task, startedAt, "리뷰를 게시하지 못했습니다", err)
	}

	reviewID := u.save(ctx, task, startedAt, review.OutcomeSucceeded, "", response, len(placed.Inline()), len(placed.Fallback()))
	if err := u.deps.Findings.SaveAll(ctx, reviewID, target, placed.Findings); err != nil {
		u.deps.Logger.Warn("지적을 저장하지 못했습니다", "target", target.Reference(), "error", err)
	}
	u.rememberHead(ctx, target)
	return nil
}

func (u *UseCase) collectFiles(ctx context.Context, target pullrequest.Target, request pullrequest.PullRequest, task job.ReviewJob) ([]pullrequest.ChangedFile, bool, error) {
	if !task.Incremental {
		files, err := u.deps.Source.ChangedFiles(ctx, target)
		return files, false, err
	}
	previous, err := u.deps.State.LastReviewedSHA(ctx, target)
	if err != nil || previous == "" || previous == request.HeadSHA {
		files, listErr := u.deps.Source.ChangedFiles(ctx, target)
		return files, false, listErr
	}
	files, err := u.deps.Source.ChangedFilesBetween(ctx, target, previous, request.HeadSHA)
	if err != nil || len(files) == 0 {
		fallback, listErr := u.deps.Source.ChangedFiles(ctx, target)
		return fallback, false, listErr
	}
	return files, true, nil
}

func (u *UseCase) loadSources(ctx context.Context, target pullrequest.Target, files []pullrequest.ChangedFile, config setting.RepoConfig) []pullrequest.ChangedFile {
	limit := config.MaxSourceChars
	if config.MaxFileChars > 0 && config.MaxFileChars < limit {
		limit = config.MaxFileChars
	}
	loaded := make([]pullrequest.ChangedFile, 0, len(files))
	for _, file := range files {
		if config.MaxFileChars > 0 && len(file.Patch) > config.MaxFileChars {
			file.Patch = file.Patch[:config.MaxFileChars]
		}
		if config.IncludeSources && limit > 0 {
			content, err := u.deps.Source.FileContent(ctx, target, file.Path, target.HeadSHA)
			if err == nil {
				if len(content) > limit {
					content = content[:limit]
					file.Truncated = true
				}
				file.Content = content
			}
		}
		file.Patch = u.deps.Masker.Mask(file.Patch)
		file.Content = u.deps.Masker.Mask(file.Content)
		loaded = append(loaded, file)
	}
	return loaded
}

func (u *UseCase) executor(target pullrequest.Target, config setting.RepoConfig) outbound.ToolExecutor {
	if config.MaxExtraReads <= 0 || u.deps.Tools == nil {
		return nil
	}
	return u.deps.Tools.ForTarget(target, target.HeadSHA, config.MaxExtraReads)
}

func (u *UseCase) publish(ctx context.Context, target pullrequest.Target, view review.SummaryView, result review.Result, attribution review.Attribution) error {
	commentID, err := u.deps.Publisher.CreateComment(ctx, target, u.deps.Renderer.SummaryBody(view))
	if err != nil {
		return err
	}

	inline := result.Inline()
	if len(inline) == 0 {
		return nil
	}

	comments := make([]review.InlineComment, 0, len(inline))
	for _, finding := range inline {
		comments = append(comments, review.InlineComment{
			Path: finding.File,
			Line: finding.Line,
			Body: u.deps.Renderer.InlineBody(finding, attribution),
		})
	}
	submitErr := u.deps.Publisher.SubmitReview(ctx, target, u.deps.Renderer.InlineReviewBody(attribution), comments)
	if submitErr == nil {
		return nil
	}
	u.deps.Logger.Warn("인라인 리뷰를 제출하지 못해 요약 코멘트에 합칩니다", "target", target.Reference(), "error", submitErr)

	degraded := view
	degraded.Fallback = append(append([]review.Finding{}, result.Fallback()...), inline...)
	degraded.InlineCount = 0
	return u.deps.Publisher.UpdateComment(ctx, target, commentID, u.deps.Renderer.SummaryBody(degraded))
}

func (u *UseCase) notify(ctx context.Context, target pullrequest.Target, task job.ReviewJob, notice review.Notice) {
	if task.Trigger.IsAutomatic() {
		return
	}
	if _, err := u.deps.Publisher.CreateComment(ctx, target, u.deps.Renderer.NoticeBody(notice)); err != nil {
		u.deps.Logger.Warn("안내 코멘트를 남기지 못했습니다", "target", target.Reference(), "error", err)
	}
}

func (u *UseCase) fail(ctx context.Context, task job.ReviewJob, startedAt time.Time, message string, cause error) error {
	u.deps.Logger.Error(message, "target", task.Target.Reference(), "error", cause)
	if notice, announce := failureNotice(task.Attempt, task.FinalAttempt, message); announce {
		u.announce(ctx, task.Target, notice)
	}
	if task.FinalAttempt {
		u.save(ctx, task, startedAt, review.OutcomeFailed, message, llm.Response{}, 0, 0)
	}
	return fmt.Errorf("%s: %w", message, cause)
}

func (u *UseCase) announce(ctx context.Context, target pullrequest.Target, notice review.Notice) {
	if _, err := u.deps.Publisher.CreateComment(ctx, target, u.deps.Renderer.NoticeBody(notice)); err != nil {
		u.deps.Logger.Warn("실패 안내를 남기지 못했습니다", "target", target.Reference(), "error", err)
	}
}

func failureNotice(attempt int, final bool, message string) (review.Notice, bool) {
	switch {
	case final:
		return review.Notice{Kind: review.NoticeFailed, Message: message + " 재시도했지만 해결되지 않아 중단합니다."}, true
	case attempt == 0:
		return review.Notice{Kind: review.NoticeRetrying, Message: message + " 잠시 후 다시 시도합니다."}, true
	default:
		return review.Notice{}, false
	}
}

func (u *UseCase) save(ctx context.Context, task job.ReviewJob, startedAt time.Time, outcome review.Outcome, detail string, response llm.Response, inline int, fallback int) uint64 {
	record := review.Record{
		Owner:         task.Target.Owner,
		Repository:    task.Target.Repository,
		Number:        task.Target.Number,
		HeadSHA:       task.Target.HeadSHA,
		Trigger:       task.Trigger,
		Outcome:       outcome,
		Provider:      response.Provider,
		Model:         response.Model,
		InlineCount:   inline,
		FallbackCount: fallback,
		Detail:        strings.TrimSpace(detail),
		StartedAt:     startedAt,
		FinishedAt:    u.deps.Clock.Now(),
	}
	id, err := u.deps.Reviews.Save(ctx, record)
	if err != nil {
		u.deps.Logger.Warn("리뷰 기록을 저장하지 못했습니다", "target", task.Target.Reference(), "error", err)
	}
	return id
}

func (u *UseCase) rememberHead(ctx context.Context, target pullrequest.Target) {
	if target.HeadSHA == "" {
		return
	}
	if err := u.deps.State.SetLastReviewedSHA(ctx, target, target.HeadSHA); err != nil {
		u.deps.Logger.Warn("마지막 리뷰 커밋을 저장하지 못했습니다", "target", target.Reference(), "error", err)
	}
}

func skipReason(task job.ReviewJob, config setting.RepoConfig) (string, bool) {
	if !task.Trigger.IsAutomatic() {
		return "", false
	}
	if !config.Sandrone.AutoReview {
		return "sandrone.autoReview가 켜져 있지 않습니다", true
	}
	if task.Trigger == review.TriggerPullRequestPushed && !config.Sandrone.AutoReviewOnPush {
		return "sandrone.autoReviewOnPush가 꺼져 있습니다", true
	}
	return "", false
}

func attributionOf(response llm.Response) review.Attribution {
	return review.Attribution{
		Provider:         response.Provider,
		Model:            response.Model,
		Label:            response.ModelLabel,
		PromptTokens:     response.Usage.PromptTokens,
		CompletionTokens: response.Usage.CompletionTokens,
		TotalTokens:      response.Usage.TotalTokens,
	}
}

func inventoryOf(reviewed []pullrequest.ChangedFile, skipped []pullrequest.ChangedFile) []pullrequest.ChangedFile {
	inventory := make([]pullrequest.ChangedFile, 0, len(reviewed)+len(skipped))
	for _, file := range reviewed {
		inventory = append(inventory, pullrequest.ChangedFile{Path: file.Path, Additions: file.Additions, Deletions: file.Deletions})
	}
	inventory = append(inventory, skipped...)
	return inventory
}

func inventoryCost(inventory []pullrequest.ChangedFile) int {
	cost := 0
	for _, file := range inventory {
		cost += len(file.Path) + 24
	}
	return cost
}

func unreviewedOf(overCount []pullrequest.ChangedFile, overflow []pullrequest.ChangedFile, failed []pullrequest.ChangedFile) []review.UnreviewedFile {
	groups := []struct {
		files  []pullrequest.ChangedFile
		reason string
	}{
		{overCount, "리뷰 대상 파일 수 상한 초과"},
		{overflow, "분량 상한 초과"},
		{failed, "모델 호출 실패"},
	}
	unreviewed := make([]review.UnreviewedFile, 0)
	for _, group := range groups {
		for _, file := range group.files {
			unreviewed = append(unreviewed, review.UnreviewedFile{
				Path:      file.Path,
				Additions: file.Additions,
				Deletions: file.Deletions,
				Reason:    group.reason,
			})
		}
	}
	sort.SliceStable(unreviewed, func(left, right int) bool {
		return unreviewed[left].Path < unreviewed[right].Path
	})
	return unreviewed
}

func batchBudget(configured int, providerBudget int) int {
	if providerBudget > 0 && (configured <= 0 || providerBudget < configured) {
		return providerBudget
	}
	return configured
}
