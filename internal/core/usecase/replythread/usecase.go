package replythread

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/job"
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
	fail := func(message string, cause error) error {
		failed, ambiguous := u.fail(ctx, task, message, cause, publicationLease)
		if ambiguous {
			releasePublicationLease = false
		}
		return failed
	}
	target := task.Target
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
			return err
		}
		publicationLease = ""
		return nil
	}
	publicationMarker := job.PublicationMarker("reply", target, task.RequestIdentity, task.CommentID, task.InThread)
	conversation, err := u.deps.Threads.Thread(ctx, target, task.CommentID)
	if err != nil {
		return fail("리뷰 스레드를 읽지 못했습니다", err)
	}
	if threadHasPublication(conversation, publicationMarker) {
		if err := u.completePublication(ctx, task.OperationKey, publicationLease); err != nil {
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
	maxCalls := u.deps.LLMMaxCalls
	if maxCalls < 1 {
		maxCalls = 12
	}
	completionRequest := llm.Request{
		Temperature:           config.Temperature,
		MaxOutputTokens:       config.MaxOutputTokens,
		Providers:             config.Sandrone.Providers,
		TaskRole:              llm.TaskRoleReply,
		DataClassification:    classification,
		ExternalCallBudget:    llm.NewExternalCallBudget(maxCalls),
		RequireCompletePrompt: true,
	}
	plan, err := u.buildReplyPromptPlan(request, conversation, source, truncated, config, task.Instruction, completionRequest)
	if err != nil {
		return fail("답글 모델 입력을 안전한 크기로 만들지 못했습니다", err)
	}
	callBudget := llm.NewDurableExternalCallBudget(maxCalls, func() bool {
		reserved, reserveErr := u.deps.Publications.ReserveReplyExternalCall(ctx, task.OperationKey, publicationLease, maxCalls)
		if reserveErr != nil {
			u.deps.Logger.Warn("답글 외부 호출 예산을 예약하지 못했습니다", "operation", task.OperationKey, "error", reserveErr)
			return false
		}
		return reserved
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
			return fail("답글 모델을 호출하지 못했습니다", errors.Join(errors.New(u.deps.Masker.Mask(err.Error())), recordErr))
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
	if err := u.renewPublication(ctx, task.OperationKey, publicationLease); err != nil {
		return err
	}
	releasePublicationLease = false
	if replyErr := u.deps.Threads.Reply(ctx, target, replyToID, replyBody); replyErr != nil {
		reconciled, reconcileErr := u.replyExists(ctx, target, replyToID, publicationMarker)
		if reconciled {
			if err := u.completePublication(ctx, task.OperationKey, publicationLease); err != nil {
				return err
			}
			publicationLease = ""
			return nil
		}
		u.deps.Logger.Error("답글을 남기지 못했습니다", "target", task.Target.Reference(), "error", errors.Join(replyErr, reconcileErr))
		return fmt.Errorf("답글을 남기지 못했습니다: %w", errors.Join(replyErr, reconcileErr))
	}
	if err := u.completePublication(ctx, task.OperationKey, publicationLease); err != nil {
		return err
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

func (u *UseCase) fail(ctx context.Context, task job.ReplyJob, message string, cause error, leaseToken string) (error, bool) {
	u.deps.Logger.Error(message, "target", task.Target.Reference(), "error", cause)
	var notice review.Notice
	markerKind := ""
	switch {
	case task.FinalAttempt:
		notice = review.Notice{Kind: review.NoticeFailed, Message: message + " 재시도했지만 해결되지 않아 중단합니다."}
		markerKind = "reply-final-notice"
	case task.Attempt == 0:
		notice = review.Notice{Kind: review.NoticeRetrying, Message: message + " 잠시 후 다시 시도합니다."}
		markerKind = "reply-retry-notice"
	}
	ambiguous := false
	if notice.Kind != "" {
		ambiguous = u.notifyFailure(ctx, task, notice, markerKind, leaseToken)
	}
	return fmt.Errorf("%s: %w", message, cause), ambiguous
}

func (u *UseCase) notifyFailure(ctx context.Context, task job.ReplyJob, notice review.Notice, markerKind string, leaseToken string) bool {
	marker := job.PublicationMarker(markerKind, task.Target, task.RequestIdentity, task.CommentID, task.InThread)
	conversation, err := u.deps.Threads.Thread(ctx, task.Target, task.CommentID)
	if err != nil {
		u.deps.Logger.Warn("기존 실패 안내를 확인하지 못했습니다", "target", task.Target.Reference(), "error", err)
		return false
	}
	if threadHasPublication(conversation, marker) {
		return false
	}
	replyToID := conversation.RootCommentID
	if replyToID == 0 {
		replyToID = task.CommentID
	}
	if err := u.renewPublication(ctx, task.OperationKey, leaseToken); err != nil {
		u.deps.Logger.Warn("실패 안내 게시 권한을 갱신하지 못했습니다", "target", task.Target.Reference(), "error", err)
		return false
	}
	body := strings.TrimRight(u.deps.Renderer.NoticeBody(notice), "\n") + "\n" + marker
	if err := u.deps.Threads.Reply(ctx, task.Target, replyToID, body); err != nil {
		reconciled, reconcileErr := u.replyExists(ctx, task.Target, replyToID, marker)
		if reconciled {
			return false
		}
		u.deps.Logger.Warn("실패 안내를 남기지 못했습니다", "target", task.Target.Reference(), "error", errors.Join(err, reconcileErr))
		return true
	}
	return false
}
