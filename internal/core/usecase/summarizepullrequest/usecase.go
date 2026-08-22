package summarizepullrequest

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/job"
	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
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

func (u *UseCase) Execute(ctx context.Context, task job.SummaryJob) error {
	startedAt := u.deps.Clock.Now()
	config, err := u.deps.Settings.RepoConfig(ctx, task.Target)
	if err != nil {
		return u.fail(ctx, task, startedAt, "설정을 읽지 못했습니다", err)
	}
	target := task.Target
	request, err := u.deps.Source.PullRequest(ctx, target)
	if err != nil {
		return u.fail(ctx, task, startedAt, "Pull Request를 읽지 못했습니다", err)
	}
	if target.HeadSHA == "" {
		target.HeadSHA = request.HeadSHA
	}
	files, err := u.deps.Source.ChangedFiles(ctx, target)
	if err != nil {
		return u.fail(ctx, task, startedAt, "변경 파일을 읽지 못했습니다", err)
	}
	chosen := selection.FileSelector{
		Include:  config.Include,
		Exclude:  config.Exclude,
		MaxFiles: config.MaxFiles,
	}.Select(files)
	if chosen.IsEmpty() {
		u.notify(ctx, target, review.Notice{Kind: review.NoticeSkipped, Message: "요약할 변경 사항이 없습니다."})
		u.save(ctx, task, startedAt, review.OutcomeSkipped, "요약 대상 파일 없음", llm.Response{})
		return nil
	}
	selected := u.trim(chosen.Files, config)

	instructions, err := u.deps.Settings.Instructions(ctx, target, config)
	if err != nil {
		u.deps.Logger.Warn("지침 문서를 읽지 못했습니다", "target", target.Reference(), "error", err)
	}

	messages := prompt.SummaryPrompt{
		Context: prompt.Context{
			PullRequest:  request,
			Files:        selected,
			Instructions: instructions,
			Config:       config,
		},
		Extra: task.Instruction,
	}.Messages()

	response, err := u.deps.Completer.Complete(ctx, llm.Request{
		Messages:        messages,
		Temperature:     config.Temperature,
		MaxOutputTokens: config.MaxOutputTokens,
		Providers:       config.Sandrone.Providers,
		ForceJSON:       true,
	}, nil)
	if err != nil {
		return u.fail(ctx, task, startedAt, "요약 모델을 호출하지 못했습니다", err)
	}
	result, err := u.deps.Parser.Parse(response.Content)
	if err != nil {
		return u.fail(ctx, task, startedAt, "모델 응답을 해석하지 못했습니다", err)
	}
	if result.Summary.IsEmpty() {
		return u.fail(ctx, task, startedAt, "모델이 요약을 만들지 못했습니다", fmt.Errorf("빈 요약"))
	}

	body := u.deps.Renderer.SummaryBody(review.SummaryView{
		Summary: result.Summary,
		Attribution: review.Attribution{
			Provider:         response.Provider,
			Model:            response.Model,
			Label:            response.ModelLabel,
			PromptTokens:     response.Usage.PromptTokens,
			CompletionTokens: response.Usage.CompletionTokens,
			TotalTokens:      response.Usage.TotalTokens,
		},
		Trigger: task.Trigger,
	})
	if err := u.place(ctx, target, config.Sandrone.SummaryPlacement, body); err != nil {
		return u.fail(ctx, task, startedAt, "요약을 게시하지 못했습니다", err)
	}
	u.save(ctx, task, startedAt, review.OutcomeSucceeded, "", response)
	return nil
}

func (u *UseCase) place(ctx context.Context, target pullrequest.Target, placement setting.SummaryPlacement, body string) error {
	switch placement {
	case setting.SummaryPlacementPullRequestBody:
		return u.deps.Publisher.UpdatePullRequestBody(ctx, target, u.deps.Renderer.Marker(), body)
	case setting.SummaryPlacementUpdateComment:
		commentID, found, err := u.deps.Publisher.FindComment(ctx, target, u.deps.Renderer.Marker())
		if err == nil && found {
			return u.deps.Publisher.UpdateComment(ctx, target, commentID, body)
		}
		_, err = u.deps.Publisher.CreateComment(ctx, target, body)
		return err
	default:
		_, err := u.deps.Publisher.CreateComment(ctx, target, body)
		return err
	}
}

func (u *UseCase) trim(files []pullrequest.ChangedFile, config setting.RepoConfig) []pullrequest.ChangedFile {
	trimmed := make([]pullrequest.ChangedFile, 0, len(files))
	for _, file := range files {
		if config.MaxFileChars > 0 && len(file.Patch) > config.MaxFileChars {
			file.Patch = file.Patch[:config.MaxFileChars]
		}
		file.Patch = u.deps.Masker.Mask(file.Patch)
		file.Content = ""
		trimmed = append(trimmed, file)
	}
	return trimmed
}

func (u *UseCase) notify(ctx context.Context, target pullrequest.Target, notice review.Notice) {
	if _, err := u.deps.Publisher.CreateComment(ctx, target, u.deps.Renderer.NoticeBody(notice)); err != nil {
		u.deps.Logger.Warn("안내 코멘트를 남기지 못했습니다", "target", target.Reference(), "error", err)
	}
}

func (u *UseCase) fail(ctx context.Context, task job.SummaryJob, startedAt time.Time, message string, cause error) error {
	u.deps.Logger.Error(message, "target", task.Target.Reference(), "error", cause)
	switch {
	case task.FinalAttempt:
		u.notify(ctx, task.Target, review.Notice{Kind: review.NoticeFailed, Message: message + " 재시도했지만 해결되지 않아 중단합니다."})
		u.save(ctx, task, startedAt, review.OutcomeFailed, message, llm.Response{})
	case task.Attempt == 0:
		u.notify(ctx, task.Target, review.Notice{Kind: review.NoticeRetrying, Message: message + " 잠시 후 다시 시도합니다."})
	}
	return fmt.Errorf("%s: %w", message, cause)
}

func (u *UseCase) save(ctx context.Context, task job.SummaryJob, startedAt time.Time, outcome review.Outcome, detail string, response llm.Response) {
	record := review.Record{
		Owner:      task.Target.Owner,
		Repository: task.Target.Repository,
		Number:     task.Target.Number,
		HeadSHA:    task.Target.HeadSHA,
		Trigger:    task.Trigger,
		Outcome:    outcome,
		Provider:   response.Provider,
		Model:      response.Model,
		Detail:     strings.TrimSpace(detail),
		StartedAt:  startedAt,
		FinishedAt: u.deps.Clock.Now(),
	}
	if _, err := u.deps.Reviews.Save(ctx, record); err != nil {
		u.deps.Logger.Warn("요약 기록을 저장하지 못했습니다", "target", task.Target.Reference(), "error", err)
	}
}
