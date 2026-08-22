package reviewpullrequest

import (
	"context"
	"fmt"
	"strings"
	"time"

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
		return u.fail(ctx, task, startedAt, "설정을 읽지 못했다", err)
	}
	if reason, skipped := skipReason(task, config); skipped {
		u.save(ctx, task, startedAt, review.OutcomeSkipped, reason, llm.Response{}, 0, 0)
		return nil
	}

	target := task.Target
	request, err := u.deps.Source.PullRequest(ctx, target)
	if err != nil {
		return u.fail(ctx, task, startedAt, "Pull Request를 읽지 못했다", err)
	}
	if target.HeadSHA == "" {
		target.HeadSHA = request.HeadSHA
	}

	files, incremental, err := u.collectFiles(ctx, target, request, task)
	if err != nil {
		return u.fail(ctx, task, startedAt, "변경 파일을 읽지 못했다", err)
	}
	selected := selection.FileSelector{
		Include:  config.Include,
		Exclude:  config.Exclude,
		MaxFiles: config.MaxFiles,
	}.Select(files)
	if len(selected) == 0 {
		u.notify(ctx, target, task, review.Notice{Kind: review.NoticeSkipped, Message: "리뷰할 변경이 없다."})
		u.save(ctx, task, startedAt, review.OutcomeSkipped, "리뷰 대상 파일 없음", llm.Response{}, 0, 0)
		u.rememberHead(ctx, target)
		return nil
	}
	selected = u.loadSources(ctx, target, selected, config)

	instructions, err := u.deps.Settings.Instructions(ctx, target, config)
	if err != nil {
		u.deps.Logger.Warn("지침 문서를 읽지 못했다", "target", target.Reference(), "error", err)
	}

	executor := u.executor(target, config)
	promptContext := prompt.Context{
		PullRequest:  request,
		Files:        selected,
		Instructions: instructions,
		Config:       config,
		Incremental:  incremental,
	}
	messages := prompt.ReviewPrompt{
		Context:      promptContext,
		Extra:        task.Instruction,
		ToolsAllowed: executor != nil,
	}.Messages()

	response, err := u.deps.Completer.Complete(ctx, llm.Request{
		Messages:        messages,
		Temperature:     config.Temperature,
		MaxOutputTokens: config.MaxOutputTokens,
		ForceJSON:       true,
	}, executor)
	if err != nil {
		return u.fail(ctx, task, startedAt, "리뷰 모델을 호출하지 못했다", err)
	}

	result, err := u.deps.Parser.Parse(response.Content)
	if err != nil {
		return u.fail(ctx, task, startedAt, "모델 응답을 해석하지 못했다", err)
	}

	findings := selection.SeverityFilter{Minimum: config.MinSeverity}.Apply(result.Findings)
	known, err := u.deps.Findings.Fingerprints(ctx, target)
	if err != nil {
		u.deps.Logger.Warn("기존 지적을 읽지 못했다", "target", target.Reference(), "error", err)
		known = map[string]struct{}{}
	}
	findings, duplicates := dedupe.DuplicateFilter{Known: known}.Apply(findings)
	findings = mapping.PositionMapper{MaxInline: config.MaxInlineComments}.Map(findings, selected)

	placed := review.Result{Summary: result.Summary, Findings: findings}
	view := review.SummaryView{
		Summary:     placed.Summary,
		Fallback:    placed.Fallback(),
		InlineCount: len(placed.Inline()),
		Provider:    response.Provider,
		Model:       response.Model,
		Trigger:     task.Trigger,
		Incremental: incremental,
		SkippedDup:  duplicates,
	}

	if err := u.publish(ctx, target, view, placed); err != nil {
		return u.fail(ctx, task, startedAt, "리뷰를 게시하지 못했다", err)
	}

	reviewID := u.save(ctx, task, startedAt, review.OutcomeSucceeded, "", response, len(placed.Inline()), len(placed.Fallback()))
	if err := u.deps.Findings.SaveAll(ctx, reviewID, target, placed.Findings); err != nil {
		u.deps.Logger.Warn("지적을 저장하지 못했다", "target", target.Reference(), "error", err)
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

func (u *UseCase) publish(ctx context.Context, target pullrequest.Target, view review.SummaryView, result review.Result) error {
	body := u.deps.Renderer.SummaryBody(view)
	inline := result.Inline()
	comments := make([]review.InlineComment, 0, len(inline))
	for _, finding := range inline {
		comments = append(comments, review.InlineComment{
			Path: finding.File,
			Line: finding.Line,
			Body: u.deps.Renderer.InlineBody(finding),
		})
	}
	submitErr := u.deps.Publisher.SubmitReview(ctx, target, body, comments)
	if submitErr == nil {
		return nil
	}
	u.deps.Logger.Warn("인라인 리뷰 제출에 실패해 요약으로 되돌린다", "target", target.Reference(), "error", submitErr)
	degraded := view
	degraded.Fallback = append(append([]review.Finding{}, result.Fallback()...), inline...)
	degraded.InlineCount = 0
	_, err := u.deps.Publisher.CreateComment(ctx, target, u.deps.Renderer.SummaryBody(degraded))
	return err
}

func (u *UseCase) notify(ctx context.Context, target pullrequest.Target, task job.ReviewJob, notice review.Notice) {
	if task.Trigger.IsAutomatic() {
		return
	}
	if _, err := u.deps.Publisher.CreateComment(ctx, target, u.deps.Renderer.NoticeBody(notice)); err != nil {
		u.deps.Logger.Warn("안내 코멘트를 남기지 못했다", "target", target.Reference(), "error", err)
	}
}

func (u *UseCase) fail(ctx context.Context, task job.ReviewJob, startedAt time.Time, message string, cause error) error {
	u.deps.Logger.Error(message, "target", task.Target.Reference(), "error", cause)
	if task.FinalAttempt {
		u.notify(ctx, task.Target, task, review.Notice{Kind: review.NoticeFailed, Message: message})
		u.save(ctx, task, startedAt, review.OutcomeFailed, message, llm.Response{}, 0, 0)
	}
	return fmt.Errorf("%s: %w", message, cause)
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
		u.deps.Logger.Warn("리뷰 기록을 저장하지 못했다", "target", task.Target.Reference(), "error", err)
	}
	return id
}

func (u *UseCase) rememberHead(ctx context.Context, target pullrequest.Target) {
	if target.HeadSHA == "" {
		return
	}
	if err := u.deps.State.SetLastReviewedSHA(ctx, target, target.HeadSHA); err != nil {
		u.deps.Logger.Warn("마지막 리뷰 커밋을 저장하지 못했다", "target", target.Reference(), "error", err)
	}
}

func skipReason(task job.ReviewJob, config setting.RepoConfig) (string, bool) {
	if !task.Trigger.IsAutomatic() {
		return "", false
	}
	if !config.Sandrone.AutoReview {
		return "sandrone.autoReview가 켜져 있지 않다", true
	}
	if task.Trigger == review.TriggerPullRequestPushed && !config.Sandrone.AutoReviewOnPush {
		return "sandrone.autoReviewOnPush가 꺼져 있다", true
	}
	return "", false
}
