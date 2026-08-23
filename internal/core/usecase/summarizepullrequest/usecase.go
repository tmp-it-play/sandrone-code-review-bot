package summarizepullrequest

import (
	"context"
	"errors"
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

const summaryPublicationLease = 30 * time.Minute
const summaryPublicationFinalizeTimeout = 5 * time.Second

func New(deps Dependencies) *UseCase {
	return &UseCase{deps: deps}
}

func (u *UseCase) Execute(ctx context.Context, task job.SummaryJob) error {
	if strings.TrimSpace(task.OperationKey) == "" || strings.TrimSpace(task.OrderKey) == "" || task.RequestedAt.IsZero() {
		u.deps.Logger.Warn("게시 순서가 없는 이전 형식의 요약 작업을 건너뜁니다", "target", task.Target.Reference())
		return nil
	}
	startedAt := u.deps.Clock.Now()
	target := task.Target
	request, err := u.deps.Source.PullRequest(ctx, target)
	if err != nil {
		return u.fail(ctx, task, startedAt, "Pull Request를 읽지 못했습니다", err)
	}
	if target.HeadSHA == "" {
		target.HeadSHA = request.HeadSHA
	}
	target.BaseRef = request.BaseRef

	config, err := u.deps.Settings.RepoConfig(ctx, target)
	if err != nil {
		return u.fail(ctx, task, startedAt, "설정을 읽지 못했습니다", err)
	}
	publicationLease := ""
	claimedAt := u.deps.Clock.Now()
	claim, claimErr := u.deps.Publications.ClaimSummaryPublication(
		ctx,
		target,
		task.OperationKey,
		task.OrderKey,
		task.RequestedAt,
		claimedAt,
		claimedAt.Add(summaryPublicationLease),
		claimedAt.Add(u.deps.Retention),
	)
	if claimErr != nil {
		return claimErr
	}
	if claim.Completed || claim.Superseded {
		return nil
	}
	publicationLease = claim.LeaseToken
	releasePublicationLease := true
	defer func() {
		if publicationLease != "" && releasePublicationLease {
			u.releasePublication(ctx, target, task.OperationKey, publicationLease)
		}
	}()
	publicationMarker := job.PublicationMarker("summary", target, task.RequestIdentity, task.CommentID, task.InThread)
	if publicationMarker != "" {
		published, publishedErr := u.publicationExists(ctx, target, publicationMarker)
		if publishedErr != nil {
			return u.fail(ctx, task, startedAt, "기존 요약 게시 결과를 확인하지 못했습니다", publishedErr)
		}
		if published {
			if completeErr := u.completePublication(ctx, target, task.OperationKey, publicationLease); completeErr != nil {
				return completeErr
			}
			publicationLease = ""
			return nil
		}
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
		if renewErr := u.renewPublication(ctx, target, task.OperationKey, publicationLease); renewErr != nil {
			return renewErr
		}
		releasePublicationLease = false
		if notifyErr := u.notify(ctx, target, review.Notice{Kind: review.NoticeSkipped, Message: "요약할 변경 사항이 없습니다."}, publicationMarker); notifyErr != nil {
			return notifyErr
		}
		if completeErr := u.completePublication(ctx, target, task.OperationKey, publicationLease); completeErr != nil {
			return completeErr
		}
		publicationLease = ""
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
	result, _, err := u.deps.Parser.Parse(response.Content)
	if err != nil {
		return u.fail(ctx, task, startedAt, "모델 응답을 해석하지 못했습니다", err)
	}
	if result.Summary.IsEmpty() {
		return u.fail(ctx, task, startedAt, "모델이 요약을 만들지 못했습니다", fmt.Errorf("빈 요약"))
	}

	body := summaryBodyWithMarker(u.deps.Renderer.SummaryBody(review.SummaryView{
		Summary: result.Summary,
		Attribution: review.Attribution{
			Provider:         response.Provider,
			Model:            response.Model,
			Label:            response.ModelLabel,
			PromptTokens:     response.Usage.PromptTokens,
			CompletionTokens: response.Usage.CompletionTokens,
			TotalTokens:      response.Usage.TotalTokens,
		},
		Style:   review.Style{Emoji: config.Emoji, Tone: string(config.Tone)},
		Trigger: task.Trigger,
	}), u.deps.Renderer.Marker(), publicationMarker)
	if renewErr := u.renewPublication(ctx, target, task.OperationKey, publicationLease); renewErr != nil {
		return renewErr
	}
	releasePublicationLease = false
	if err := u.place(ctx, target, config.Sandrone.SummaryPlacement, publicationMarker, body); err != nil {
		return u.fail(ctx, task, startedAt, "요약을 게시하지 못했습니다", err)
	}
	if completeErr := u.completePublication(ctx, target, task.OperationKey, publicationLease); completeErr != nil {
		return completeErr
	}
	publicationLease = ""
	u.save(ctx, task, startedAt, review.OutcomeSucceeded, "", response)
	return nil
}

func (u *UseCase) renewPublication(ctx context.Context, target pullrequest.Target, operationKey string, leaseToken string) error {
	if operationKey == "" || leaseToken == "" {
		return nil
	}
	return u.deps.Publications.RenewSummaryPublication(ctx, target, operationKey, leaseToken, u.deps.Clock.Now().Add(summaryPublicationLease))
}

func (u *UseCase) completePublication(ctx context.Context, target pullrequest.Target, operationKey string, leaseToken string) error {
	if operationKey == "" || leaseToken == "" {
		return nil
	}
	now := u.deps.Clock.Now()
	return u.deps.Publications.CompleteSummaryPublication(ctx, target, operationKey, leaseToken, now, now.Add(u.deps.Retention))
}

func (u *UseCase) releasePublication(ctx context.Context, target pullrequest.Target, operationKey string, leaseToken string) {
	releaseContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), summaryPublicationFinalizeTimeout)
	defer cancel()
	if err := u.deps.Publications.ReleaseSummaryPublication(releaseContext, target, operationKey, leaseToken); err != nil {
		u.deps.Logger.Warn("요약 게시 lease를 해제하지 못했습니다", "target", target.Reference(), "error", err)
	}
}

func (u *UseCase) place(ctx context.Context, target pullrequest.Target, placement setting.SummaryPlacement, publicationMarker string, body string) error {
	var publishErr error
	switch placement {
	case setting.SummaryPlacementPullRequestBody:
		publishErr = u.deps.Publisher.UpdatePullRequestBody(ctx, target, u.deps.Renderer.Marker(), body)
	case setting.SummaryPlacementUpdateComment:
		commentID, found, err := u.deps.Publisher.FindComment(ctx, target, u.deps.Renderer.Marker())
		if err != nil {
			return err
		}
		if found {
			publishErr = u.deps.Publisher.UpdateComment(ctx, target, commentID, body)
		} else {
			_, publishErr = u.deps.Publisher.CreateComment(ctx, target, body)
		}
	default:
		_, publishErr = u.deps.Publisher.CreateComment(ctx, target, body)
	}
	if publishErr == nil || publicationMarker == "" {
		return publishErr
	}
	published, reconcileErr := u.publicationExists(ctx, target, publicationMarker)
	if published {
		return nil
	}
	return errors.Join(publishErr, reconcileErr)
}

func (u *UseCase) publicationExists(ctx context.Context, target pullrequest.Target, marker string) (bool, error) {
	if marker == "" {
		return false, nil
	}
	request, bodyErr := u.deps.Source.PullRequest(ctx, target)
	if bodyErr == nil && strings.Contains(request.Body, marker) {
		return true, nil
	}
	_, found, commentErr := u.deps.Publisher.FindComment(ctx, target, marker)
	if found {
		return true, nil
	}
	return false, errors.Join(bodyErr, commentErr)
}

func summaryBodyWithMarker(body string, sectionMarker string, publicationMarker string) string {
	if publicationMarker == "" {
		return body
	}
	closing := strings.Replace(sectionMarker, "<!-- ", "<!-- /", 1)
	index := strings.LastIndex(body, closing)
	if index < 0 {
		return strings.TrimRight(body, "\n") + "\n" + publicationMarker
	}
	return body[:index] + publicationMarker + "\n" + body[index:]
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

func (u *UseCase) notify(ctx context.Context, target pullrequest.Target, notice review.Notice, marker string) error {
	if marker != "" {
		_, found, err := u.deps.Publisher.FindComment(ctx, target, marker)
		if err != nil || found {
			return err
		}
	}
	body := u.deps.Renderer.NoticeBody(notice)
	if marker != "" {
		body = strings.TrimRight(body, "\n") + "\n" + marker
	}
	_, createErr := u.deps.Publisher.CreateComment(ctx, target, body)
	if createErr == nil || marker == "" {
		return createErr
	}
	_, found, reconcileErr := u.deps.Publisher.FindComment(ctx, target, marker)
	if found {
		return nil
	}
	return errors.Join(createErr, reconcileErr)
}

func (u *UseCase) fail(ctx context.Context, task job.SummaryJob, startedAt time.Time, message string, cause error) error {
	u.deps.Logger.Error(message, "target", task.Target.Reference(), "error", cause)
	switch {
	case task.FinalAttempt:
		marker := job.PublicationMarker("summary-final-notice", task.Target, task.RequestIdentity, task.CommentID, task.InThread)
		if err := u.notify(ctx, task.Target, review.Notice{Kind: review.NoticeFailed, Message: message + " 재시도했지만 해결되지 않아 중단합니다."}, marker); err != nil {
			u.deps.Logger.Warn("요약 실패 안내를 남기지 못했습니다", "target", task.Target.Reference(), "error", err)
		}
		u.save(ctx, task, startedAt, review.OutcomeFailed, message, llm.Response{})
	case task.Attempt == 0:
		marker := job.PublicationMarker("summary-retry-notice", task.Target, task.RequestIdentity, task.CommentID, task.InThread)
		if err := u.notify(ctx, task.Target, review.Notice{Kind: review.NoticeRetrying, Message: message + " 잠시 후 다시 시도합니다."}, marker); err != nil {
			u.deps.Logger.Warn("요약 재시도 안내를 남기지 못했습니다", "target", task.Target.Reference(), "error", err)
		}
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
