package replythread

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/job"
	"github.com/it-play/sandrone-code-review-bot/internal/core/leaseheartbeat"
	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
	"github.com/it-play/sandrone-code-review-bot/internal/core/publication"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"github.com/it-play/sandrone-code-review-bot/internal/core/thread"
)

type UseCase struct {
	deps Dependencies
}

const replyPublicationLease = 30 * time.Minute
const replyPublicationFinalizeTimeout = 5 * time.Second
const replyPublicationRecoveryDelay = replyPublicationLease + 10*time.Second

func New(deps Dependencies) *UseCase {
	return &UseCase{deps: deps}
}

func (u *UseCase) Execute(ctx context.Context, task job.ReplyJob) error {
	if strings.TrimSpace(task.OperationKey) == "" {
		task.OperationKey = job.OperationKey("reply", task.Target, task.RequestIdentity, task.CommentID, task.InThread)
	}
	if strings.TrimSpace(task.OperationKey) == "" || task.CommentID <= 0 {
		u.deps.Logger.Warn("이전 형식의 답글 작업에 안전한 게시 식별자를 만들 수 없습니다", "target", task.Target.Reference())
		return nil
	}
	task.Instruction = u.deps.Masker.Mask(task.Instruction)
	claimedAt := u.deps.Clock.Now()
	claim, claimErr := u.deps.Publications.ClaimReplyPublication(
		ctx,
		task.Target,
		task.OperationKey,
		task.CommentID,
		task.InThread,
		claimedAt,
		claimedAt.Add(replyPublicationLease),
		claimedAt.Add(u.deps.Retention),
	)
	if claimErr != nil {
		return claimErr
	}
	if claim.Completed {
		return nil
	}
	publicationLease := claim.LeaseToken
	releasePublicationLease := true
	defer func() {
		if publicationLease != "" && releasePublicationLease {
			u.releasePublication(ctx, task.OperationKey, publicationLease)
		}
	}()
	target := task.Target
	withHeartbeat := func(work func(context.Context) error) error {
		return leaseheartbeat.Run(ctx, replyPublicationLease, func(heartbeatContext context.Context) error {
			return u.renewPublication(heartbeatContext, task.OperationKey, publicationLease)
		}, work)
	}
	terminalFailure := func(message string, cause error) error {
		ambiguous := false
		notifyErr := withHeartbeat(func(notificationContext context.Context) error {
			var failureErr error
			failureErr, ambiguous = u.notifyFailure(notificationContext, task, review.Notice{Kind: review.NoticeFailed, Message: message}, "reply-final-notice", publicationLease)
			return failureErr
		})
		if notifyErr != nil {
			failed := errors.Join(cause, notifyErr)
			if task.FinalAttempt {
				u.deps.Logger.Warn("최종 답글 실패 안내를 남기지 못해 lease만 해제합니다", "target", task.Target.Reference(), "error", failed)
				releasePublicationLease = true
				return failed
			}
			if ambiguous {
				releasePublicationLease = false
				return &job.RetryAtError{At: u.deps.Clock.Now().Add(replyPublicationRecoveryDelay), Cause: failed}
			}
			return &job.RetryAtError{At: u.deps.Clock.Now().Add(time.Minute), Cause: failed}
		}
		if completeErr := u.completePublication(ctx, task.OperationKey, publicationLease); completeErr != nil {
			if task.FinalAttempt {
				u.deps.Logger.Warn("최종 답글 실패 안내의 완료 상태를 저장하지 못해 lease만 해제합니다", "target", task.Target.Reference(), "error", completeErr)
				releasePublicationLease = true
				return nil
			}
			releasePublicationLease = false
			return &job.RetryAtError{At: u.deps.Clock.Now().Add(replyPublicationRecoveryDelay), Cause: completeErr}
		}
		publicationLease = ""
		return nil
	}
	fail := func(message string, cause error) error {
		if task.FinalizationAttempt {
			return terminalFailure(message+" 재시도했지만 해결되지 않아 중단합니다.", cause)
		}
		u.deps.Logger.Error(message, "target", task.Target.Reference(), "error", cause)
		failed := fmt.Errorf("%s: %w", message, cause)
		if task.Attempt != 0 {
			return failed
		}
		ambiguous := false
		notifyErr := withHeartbeat(func(notificationContext context.Context) error {
			var failureErr error
			failureErr, ambiguous = u.notifyFailure(notificationContext, task, review.Notice{Kind: review.NoticeRetrying, Message: message + " 잠시 후 다시 시도합니다."}, "reply-retry-notice", publicationLease)
			return failureErr
		})
		failed = errors.Join(failed, notifyErr)
		if ambiguous {
			releasePublicationLease = false
			return &job.RetryAtError{At: u.deps.Clock.Now().Add(replyPublicationRecoveryDelay), Cause: failed}
		}
		return failed
	}
	request, err := u.deps.Source.PullRequest(ctx, target)
	if err != nil {
		return fail("Pull Request를 읽지 못했습니다", err)
	}
	request = request.Masked(u.deps.Masker.Mask)
	if target.HeadSHA == "" {
		target.HeadSHA = request.HeadSHA
	}
	target.BaseSHA = request.BaseSHA
	target.BaseRef = request.BaseRef

	config, err := u.deps.Settings.RepoConfig(ctx, target)
	if err != nil {
		return fail("설정을 읽지 못했습니다", err)
	}
	if !config.ThreadReply {
		if err := u.completePublication(ctx, task.OperationKey, publicationLease); err != nil {
			if task.FinalAttempt {
				u.deps.Logger.Warn("비활성화된 답글 작업의 완료 상태를 저장하지 못해 lease만 해제합니다", "target", task.Target.Reference(), "error", err)
				releasePublicationLease = true
				return nil
			}
			return err
		}
		publicationLease = ""
		return nil
	}
	publicationMarker := job.PublicationMarker("reply", target, task.RequestIdentity, task.CommentID, task.InThread)
	conversation := thread.Thread{}
	err = withHeartbeat(func(threadContext context.Context) error {
		var threadErr error
		conversation, threadErr = u.deps.Threads.Thread(threadContext, target, task.CommentID)
		return threadErr
	})
	if err != nil {
		return fail("리뷰 스레드를 읽지 못했습니다", err)
	}
	if threadHasPublication(conversation, publicationMarker) {
		if err := u.completePublication(ctx, task.OperationKey, publicationLease); err != nil {
			if task.FinalAttempt {
				u.deps.Logger.Warn("기존 답글 marker의 완료 상태를 저장하지 못해 lease만 해제합니다", "target", task.Target.Reference(), "error", err)
				releasePublicationLease = true
				return nil
			}
			return err
		}
		publicationLease = ""
		return nil
	}
	failureMarker := job.PublicationMarker("reply-final-notice", target, task.RequestIdentity, task.CommentID, task.InThread)
	if threadHasPublication(conversation, failureMarker) {
		if err := u.completePublication(ctx, task.OperationKey, publicationLease); err != nil {
			if task.FinalAttempt {
				u.deps.Logger.Warn("기존 답글 실패 marker의 완료 상태를 저장하지 못해 lease만 해제합니다", "target", task.Target.Reference(), "error", err)
				releasePublicationLease = true
				return nil
			}
			return err
		}
		publicationLease = ""
		return nil
	}
	replyToID := conversation.RootCommentID
	if replyToID == 0 {
		replyToID = task.CommentID
	}

	source, truncated := u.currentSource(ctx, task, conversation.Path, target.HeadSHA, config.MaxSourceChars)
	classification := llm.DataClassificationPublic
	if request.Private {
		classification = llm.DataClassificationPrivateCode
	}
	maxCalls := llm.OperationCallLimit(u.deps.LLMMaxCalls, llm.TaskRoleReply)
	completionRequest := llm.Request{
		Temperature:           config.Temperature,
		MaxOutputTokens:       config.MaxOutputTokens,
		Providers:             config.Sandrone.Providers,
		TaskRole:              llm.TaskRoleReply,
		DataClassification:    classification,
		ExternalCallBudget:    llm.NewExternalCallBudget(maxCalls),
		RequireCompletePrompt: true,
		ResponseValidation:    llm.ResponseValidation{Policy: llm.ResponseValidationNonEmpty},
	}
	plan, err := u.buildReplyPromptPlan(request, conversation, source, truncated, config, task.Instruction, completionRequest)
	if err != nil {
		if errors.Is(err, llm.ErrPromptCapacity) {
			return terminalFailure("답글 입력이 사용 가능한 모델 한도를 넘어 이번 요청을 종료합니다.", err)
		}
		return fail("답글 모델 입력을 안전한 크기로 만들지 못했습니다", err)
	}
	callBudget := llm.NewScopedDurableExternalCallBudget(maxCalls, claim.ExternalCalls, maxCalls, func() error {
		reservedAt := u.deps.Clock.Now()
		reserved, reserveErr := u.deps.Publications.ReserveReplyExternalCall(ctx, task.OperationKey, publicationLease, maxCalls, reservedAt, reservedAt.Add(replyPublicationLease))
		if reserveErr != nil {
			return reserveErr
		}
		if !reserved {
			return llm.ErrExternalCallBudgetExhausted
		}
		return nil
	})

	completionRequest.Messages = plan.Messages
	completionRequest.ExternalCallBudget = callBudget
	inputHash, err := llm.CompletionInputHash(completionRequest, u.deps.Completer.PolicyHashInputs(completionRequest), "reply-canonical-v2")
	if err != nil {
		return fail("답글 모델 입력 식별자를 만들지 못했습니다", err)
	}
	checkpoint, cached, err := u.deps.Publications.ReplyCompletion(ctx, task.OperationKey, publicationLease, inputHash, u.deps.Clock.Now())
	if err != nil {
		return fail("기존 답글 모델 결과를 읽지 못했습니다", err)
	}
	response := checkpoint.MetadataResponse()
	body := checkpoint.CanonicalContent
	if !cached {
		response, err = u.deps.Completer.Complete(ctx, completionRequest, nil)
		response.Content = u.deps.Masker.Mask(response.Content)
		if err != nil {
			_, recordErr := u.recordCompletionAttempt(ctx, task.OperationKey, publicationLease, response)
			maskedErr := &llm.SanitizedError{Message: u.deps.Masker.Mask(err.Error()), Cause: err}
			if failure, ok := llm.AsCompletionFailure(err); ok && recordErr == nil {
				if !failure.Terminal() && !task.FinalizationAttempt {
					u.deps.Logger.Info("일시적인 답글 모델 실패를 사용자 실패로 처리하지 않고 재개를 예약합니다", "target", task.Target.Reference(), "retry_at", failure.RetryAt())
					return maskedErr
				}
				return terminalFailure("답글 모델이 유효한 결과를 만들지 못해 이번 요청을 종료합니다.", maskedErr)
			}
			return fail("답글 모델을 호출하지 못했습니다", errors.Join(maskedErr, recordErr))
		}
		if !response.Completed() {
			_, recordErr := u.recordCompletionAttempt(ctx, task.OperationKey, publicationLease, response)
			return fail("답글 모델 응답이 완료되지 않았습니다", errors.Join(fmt.Errorf("finish reason: %s", response.FinishReason), recordErr))
		}
		body = strings.TrimSpace(response.Content)
		if body == "" {
			_, recordErr := u.recordCompletionAttempt(ctx, task.OperationKey, publicationLease, response)
			resultErr := errors.Join(errors.New("빈 응답"), recordErr)
			return fail("모델이 답글을 만들지 못했습니다", resultErr)
		}
		checkpointContext, checkpointCancel := completionCheckpointContext(ctx)
		checkpoint, err = u.deps.Publications.SaveReplyCompletion(
			checkpointContext,
			task.OperationKey,
			publicationLease,
			publication.NewCompletionCheckpoint(inputHash, body, response, u.deps.Clock.Now()),
		)
		checkpointCancel()
		if err != nil {
			return fail("답글 모델 결과를 저장하지 못했습니다", err)
		}
		response = checkpoint.MetadataResponse()
	}
	replyBody := u.deps.Renderer.ReplyBody(body, review.Attribution{
		Provider:         response.Provider,
		Model:            response.Model,
		Label:            response.ModelLabel,
		PromptTokens:     response.Usage.PromptTokens,
		CompletionTokens: response.Usage.CompletionTokens,
		TotalTokens:      response.Usage.TotalTokens,
	})
	if publicationMarker != "" {
		replyBody = strings.TrimRight(replyBody, "\n") + "\n" + publicationMarker
	}
	releasePublicationLease = false
	publicationErr := withHeartbeat(func(publicationContext context.Context) error {
		replyErr := u.deps.Threads.Reply(publicationContext, target, replyToID, replyBody)
		if replyErr == nil {
			return nil
		}
		reconciled, reconcileErr := u.replyExists(publicationContext, target, replyToID, publicationMarker)
		if reconciled {
			return nil
		}
		return fmt.Errorf("답글을 남기지 못했습니다: %w", errors.Join(replyErr, reconcileErr))
	})
	if publicationErr != nil {
		u.deps.Logger.Error("답글을 남기지 못했습니다", "target", task.Target.Reference(), "error", publicationErr)
		if task.FinalAttempt {
			return terminalFailure("답글을 게시하지 못해 이번 요청을 종료합니다.", publicationErr)
		}
		return &job.RetryAtError{At: u.deps.Clock.Now().Add(replyPublicationRecoveryDelay), Cause: publicationErr}
	}
	if err := u.completePublication(ctx, task.OperationKey, publicationLease); err != nil {
		if task.FinalAttempt {
			u.deps.Logger.Warn("게시된 답글의 완료 상태를 저장하지 못해 lease만 해제합니다", "target", task.Target.Reference(), "error", err)
			releasePublicationLease = true
			return nil
		}
		return &job.RetryAtError{At: u.deps.Clock.Now().Add(replyPublicationRecoveryDelay), Cause: err}
	}
	publicationLease = ""
	return nil
}

func (u *UseCase) recordCompletionAttempt(ctx context.Context, operationKey string, leaseToken string, response llm.Response) (llm.Response, error) {
	response.Content = u.deps.Masker.Mask(response.Content)
	checkpointContext, checkpointCancel := completionCheckpointContext(ctx)
	defer checkpointCancel()
	checkpoint, err := u.deps.Publications.RecordReplyCompletionAttempt(checkpointContext, operationKey, leaseToken, response, u.deps.Clock.Now())
	if err != nil {
		return response, err
	}
	return checkpoint.MetadataResponse(), nil
}

func (u *UseCase) renewPublication(ctx context.Context, operationKey string, leaseToken string) error {
	return u.deps.Publications.RenewReplyPublication(ctx, operationKey, leaseToken, u.deps.Clock.Now().Add(replyPublicationLease))
}

func (u *UseCase) completePublication(ctx context.Context, operationKey string, leaseToken string) error {
	now := u.deps.Clock.Now()
	return u.deps.Publications.CompleteReplyPublication(ctx, operationKey, leaseToken, now)
}

func (u *UseCase) releasePublication(ctx context.Context, operationKey string, leaseToken string) {
	releaseContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), replyPublicationFinalizeTimeout)
	defer cancel()
	if err := u.deps.Publications.ReleaseReplyPublication(releaseContext, operationKey, leaseToken); err != nil {
		u.deps.Logger.Warn("답글 게시 lease를 해제하지 못했습니다", "operation", operationKey, "error", err)
	}
}

func (u *UseCase) replyExists(ctx context.Context, target pullrequest.Target, commentID int64, marker string) (bool, error) {
	if marker == "" {
		return false, nil
	}
	conversation, err := u.deps.Threads.Thread(ctx, target, commentID)
	if err != nil {
		return false, err
	}
	return threadHasPublication(conversation, marker), nil
}

func threadHasPublication(conversation thread.Thread, marker string) bool {
	if marker == "" {
		return false
	}
	for _, message := range conversation.Messages {
		if message.FromBot && strings.Contains(message.Body, marker) {
			return true
		}
	}
	return false
}

func (u *UseCase) currentSource(ctx context.Context, task job.ReplyJob, path string, ref string, limit int) (string, bool) {
	if strings.TrimSpace(path) == "" {
		return "", false
	}
	content, err := u.deps.Source.FileContent(ctx, task.Target, path, ref)
	if err != nil {
		u.deps.Logger.Warn("최신 파일 내용을 읽지 못했습니다", "target", task.Target.Reference(), "path", path, "error", err)
		return "", false
	}
	content = u.deps.Masker.Mask(content)
	if limit > 0 && len(content) > limit {
		return truncateReplyUTF8(content, limit), true
	}
	return content, false
}

func (u *UseCase) notifyFailure(ctx context.Context, task job.ReplyJob, notice review.Notice, markerKind string, leaseToken string) (error, bool) {
	marker := job.PublicationMarker(markerKind, task.Target, task.RequestIdentity, task.CommentID, task.InThread)
	conversation, err := u.deps.Threads.Thread(ctx, task.Target, task.CommentID)
	if err != nil {
		return fmt.Errorf("기존 실패 안내를 확인하지 못했습니다: %w", err), false
	}
	if threadHasPublication(conversation, marker) {
		return nil, false
	}
	replyToID := conversation.RootCommentID
	if replyToID == 0 {
		replyToID = task.CommentID
	}
	if err := u.renewPublication(ctx, task.OperationKey, leaseToken); err != nil {
		return fmt.Errorf("실패 안내 게시 권한을 갱신하지 못했습니다: %w", err), false
	}
	body := strings.TrimRight(u.deps.Renderer.NoticeBody(notice), "\n") + "\n" + marker
	if err := u.deps.Threads.Reply(ctx, task.Target, replyToID, body); err != nil {
		reconciled, reconcileErr := u.replyExists(ctx, task.Target, replyToID, marker)
		if reconciled {
			return nil, false
		}
		return fmt.Errorf("실패 안내를 남기지 못했습니다: %w", errors.Join(err, reconcileErr)), true
	}
	return nil, false
}
