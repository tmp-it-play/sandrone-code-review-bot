package reviewpullrequest

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/it-play/sandrone-code-review-bot/internal/core/batching"
	"github.com/it-play/sandrone-code-review-bot/internal/core/instruction"
	"github.com/it-play/sandrone-code-review-bot/internal/core/job"
	"github.com/it-play/sandrone-code-review-bot/internal/core/leaseheartbeat"
	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
	"github.com/it-play/sandrone-code-review-bot/internal/core/port/outbound"
	"github.com/it-play/sandrone-code-review-bot/internal/core/publication"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
	"github.com/it-play/sandrone-code-review-bot/internal/core/selection"
	"github.com/it-play/sandrone-code-review-bot/internal/core/setting"
	"github.com/it-play/sandrone-code-review-bot/internal/core/usecase/reviewpublication"
)

const reactionTimeout = 5 * time.Second
const reviewRunLease = 20 * time.Minute
const reviewPublicationLease = 30 * time.Minute
const reviewPublicationRecoveryDelay = 5*time.Minute + 10*time.Second

type UseCase struct {
	deps            Dependencies
	publications    *reviewpublication.PreparedPublisher
	unitExecutor    *reviewUnitExecutor
	resultFinalizer *reviewResultFinalizer
}

func New(deps Dependencies) *UseCase {
	verifier := newFindingVerifier(findingVerifierDependencies{
		verification: deps.Verification,
		completer:    deps.Completer,
		masker:       deps.Masker,
		clock:        deps.Clock,
		logger:       deps.Logger,
	})
	return &UseCase{
		deps: deps,
		publications: reviewpublication.New(reviewpublication.Dependencies{
			Publisher: deps.Publisher,
			Receipts:  deps.Publications,
			Ownership: deps.Runs,
			Checks:    deps.Checks,
			Clock:     deps.Clock,
			Logger:    deps.Logger,
		}),
		unitExecutor: newReviewUnitExecutor(reviewUnitExecutorDependencies{
			execution: deps.Execution,
			completer: deps.Completer,
			tools:     deps.Tools,
			masker:    deps.Masker,
			clock:     deps.Clock,
			parser:    deps.Parser,
			logger:    deps.Logger,
		}),
		resultFinalizer: newReviewResultFinalizer(reviewResultFinalizerDependencies{
			findings: deps.Findings,
			verifier: verifier,
			logger:   deps.Logger,
		}),
	}
}

func (u *UseCase) Execute(ctx context.Context, task job.ReviewJob) error {
	_, err := u.ExecuteWithOutcome(ctx, task)
	return err
}

func (u *UseCase) ExecuteWithOutcome(ctx context.Context, task job.ReviewJob) (review.Outcome, error) {
	outcome := review.OutcomeUnavailable
	err := u.execute(ctx, task, &outcome)
	return outcome, err
}

func (u *UseCase) execute(ctx context.Context, task job.ReviewJob, outcome *review.Outcome) (executeErr error) {
	activeRunID := uint64(0)
	activeRunLease := ""
	workflowRunCreated := false
	workflowRunID := uint64(0)
	publicationClaimed := false
	publicationWasResumed := false
	publicationCompleted := false
	publicationEffectsStarted := false
	startedAt := u.deps.Clock.Now()
	progress := requestProgress(task)
	failureResponse := llm.Response{}
	fail := func(message string, cause error) error {
		if errors.Is(cause, reviewworkflow.ErrRunSuperseded) && activeRunID != 0 && activeRunLease != "" && !publicationClaimed {
			detail := "더 최신인 PR 상태가 현재 리뷰 실행을 대체했습니다"
			supersedingHeadSHA := u.resolveSupersedingHeadSHA(ctx, task.Target)
			actualStatus, finishErr := u.finishWorkflowSupersededByHeadWithProgress(ctx, activeRunID, activeRunLease, supersedingHeadSHA, detail, progress)
			if finishErr != nil {
				return fmt.Errorf("대체된 리뷰 실행을 종료하지 못했습니다: %w", errors.Join(cause, finishErr))
			}
			if actualStatus != reviewworkflow.RunStatusSuperseded {
				*outcome = reviewOutcomeOf(actualStatus)
				return nil
			}
			if notifyErr := u.notify(ctx, task.Target, task, progress, review.Notice{Kind: review.NoticeSuperseded, Message: "더 최신 변경이 감지되어 이 리뷰를 종료했습니다. 최신 리뷰 실행이 이어서 처리합니다."}); notifyErr != nil {
				return fmt.Errorf("대체된 리뷰의 진행 코멘트를 정리하지 못했습니다: %w", errors.Join(cause, notifyErr))
			}
			u.save(ctx, task, startedAt, review.OutcomeSuperseded, detail, failureResponse, 0, 0, activeRunID)
			*outcome = reviewOutcomeOf(actualStatus)
			return nil
		}
		if publicationClaimed {
			if task.FinalAttempt && !publicationEffectsStarted && !publicationWasResumed {
				detail := u.deps.Masker.Mask(message + ": " + cause.Error())
				actualStatus, finishErr := u.finishWorkflowWithOutcomeAndProgress(ctx, activeRunID, activeRunLease, reviewworkflow.RunStatusFailed, review.OutcomeFailed, detail, false, progress)
				if finishErr != nil {
					return fmt.Errorf("게시 외부 효과 전 실패한 리뷰를 종료하지 못했습니다: %w", errors.Join(cause, finishErr))
				}
				publicationClaimed = false
				activeRunLease = ""
				if actualStatus != reviewworkflow.RunStatusFailed {
					if reconcileErr := u.reconcileTerminalProgress(ctx, task.Target, reviewworkflow.Run{ID: activeRunID, Status: actualStatus}, progress); reconcileErr != nil {
						return fmt.Errorf("이미 종료된 리뷰의 진행 코멘트를 정리하지 못했습니다: %w", reconcileErr)
					}
					*outcome = reviewOutcomeOf(actualStatus)
					return nil
				}
				return u.fail(ctx, task, startedAt, message, cause, activeRunID, true, failureResponse, progress)
			}
			u.deps.Logger.Error(message, "target", task.Target.Reference(), "error", cause)
			if notice, announce := failureNotice(task.Attempt, task.FinalAttempt); announce && !publicationEffectsStarted && !publicationCompleted {
				if announceErr := u.announce(ctx, task.Target, task, progress, notice); announceErr != nil {
					u.deps.Logger.Warn("리뷰 게시 실패 안내를 남기지 못했습니다", "target", task.Target.Reference(), "error", announceErr)
				}
			}
			failed := fmt.Errorf("%s: %w", message, cause)
			if task.FinalAttempt {
				return failed
			}
			var scheduled interface{ RetryAt() time.Time }
			if errors.As(failed, &scheduled) && !scheduled.RetryAt().IsZero() {
				return failed
			}
			return &job.RetryAtError{At: u.deps.Clock.Now().Add(reviewPublicationRecoveryDelay), Cause: failed}
		}
		failureRunID := activeRunID
		if task.FinalAttempt && failureRunID == 0 && progress != nil {
			persistedRunID, terminalStatus, persistErr := u.finishUnleasedProgressFailure(ctx, task, startedAt, u.deps.Masker.Mask(message+": "+cause.Error()), workflowRunID, progress)
			if persistErr != nil {
				return fmt.Errorf("최종 실패한 진행 코멘트 정리를 영속화하지 못했습니다: %w", errors.Join(cause, persistErr))
			}
			failureRunID = persistedRunID
			workflowRunID = persistedRunID
			if terminalStatus != reviewworkflow.RunStatusFailed {
				if reconcileErr := u.reconcileTerminalProgress(ctx, task.Target, reviewworkflow.Run{ID: persistedRunID, Status: terminalStatus}, progress); reconcileErr != nil {
					return fmt.Errorf("이미 종료된 리뷰의 진행 코멘트를 정리하지 못했습니다: %w", reconcileErr)
				}
				*outcome = reviewOutcomeOf(terminalStatus)
				return nil
			}
		}
		recordFailure := !workflowRunCreated || failureRunID != 0
		return u.fail(ctx, task, startedAt, message, cause, failureRunID, recordFailure, failureResponse, progress)
	}
	defer func() {
		progress.Stop()
		if executeErr != nil && !task.FinalAttempt {
			fenceContext, fenceCancel := context.WithTimeout(context.WithoutCancel(ctx), reviewworkflow.PublicationInvalidationFenceDelay+progressUpdateTimeout)
			if err := u.waitForProgressMutationFence(fenceContext, task.Target, progress); err != nil {
				u.deps.Logger.Warn("불확실한 진행 코멘트 요청의 안전 구간을 기다리지 못했습니다", "run", activeRunID, "error", err)
			}
			fenceCancel()
		}
		if activeRunID == 0 || activeRunLease == "" {
			return
		}
		finishContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), reactionTimeout)
		defer cancel()
		if executeErr != nil && task.FinalAttempt && !publicationClaimed {
			if _, err := u.finishWorkflowWithOutcomeAndProgress(finishContext, activeRunID, activeRunLease, reviewworkflow.RunStatusFailed, review.OutcomeFailed, u.deps.Masker.Mask(executeErr.Error()), false, progress); err != nil {
				u.deps.Logger.Error("실패한 리뷰 실행을 종료하지 못했습니다", "run", activeRunID, "error", err)
			}
		}
		if err := u.deps.Runs.ReleaseRun(finishContext, activeRunID, activeRunLease, u.deps.Clock.Now()); err != nil {
			u.deps.Logger.Warn("리뷰 실행 lease를 해제하지 못했습니다", "run", activeRunID, "error", err)
		}
	}()
	if progress != nil && u.deps.Progress != nil {
		if err := u.deps.Progress.RecoverReviewProgress(ctx, task); err != nil {
			return fail("진행 코멘트 생성을 복구하지 못했습니다", err)
		}
	}
	task.Instruction = u.deps.Masker.Mask(task.Instruction)
	runTarget := task.Target
	request, err := u.deps.Source.PullRequest(ctx, runTarget)
	if err != nil {
		return fail("Pull Request를 읽지 못했습니다", err)
	}
	request = request.Masked(u.deps.Masker.Mask)
	if task.SnapshotObservedAt.IsZero() {
		task.SnapshotObservedAt = request.UpdatedAt
		if task.SnapshotObservedAt.IsZero() {
			task.SnapshotObservedAt = startedAt
		}
	}
	if strings.TrimSpace(task.SnapshotOrderKey) == "" {
		if task.CommentID > 0 {
			task.SnapshotOrderKey = job.OrderKey(task.CommentID, task.InThread)
		} else if strings.TrimSpace(task.RequestIdentity) != "" {
			task.SnapshotOrderKey = task.RequestIdentity
		} else {
			task.SnapshotOrderKey = "legacy:automatic"
		}
	}
	snapshotObservedAt := task.SnapshotObservedAt
	activityBoundary := task.RequestReceivedAt
	if activityBoundary.IsZero() {
		activityBoundary = snapshotObservedAt
	}
	if activityBoundary.IsZero() {
		activityBoundary = startedAt
	}
	staleHead := runTarget.HeadSHA != "" && runTarget.HeadSHA != request.HeadSHA
	if runTarget.HeadSHA == "" {
		runTarget.HeadSHA = request.HeadSHA
	}
	if runTarget.BaseSHA == "" || !staleHead {
		runTarget.BaseSHA = request.BaseSHA
	}
	runTarget.BaseRef = request.BaseRef
	target, targetMatches, targetMatchErr := u.rebindNoopHeadChange(ctx, runTarget, request)
	if targetMatchErr != nil {
		return fail("현재 head 이동의 실제 파일 변경을 확인하지 못했습니다", targetMatchErr)
	}
	stale := !targetMatches
	task.Target = target

	config, err := u.deps.Settings.RepoConfig(ctx, target)
	if err != nil {
		return fail("설정을 읽지 못했습니다", err)
	}
	classification := llm.DataClassificationPublic
	if request.Private {
		classification = llm.DataClassificationPrivateCode
	}
	maxCalls := llm.OperationCallLimit(u.deps.LLMMaxCalls, llm.TaskRoleReviewer)
	verifierCallReserve := 0
	if maxCalls > 1 {
		verifierCallReserve = 1
	}
	reviewerCallLimit := maxCalls - verifierCallReserve
	callBudget := llm.NewExternalCallBudget(maxCalls)
	reviewRequest := llm.Request{
		Temperature:           config.Temperature,
		MaxOutputTokens:       reviewOutputTokens(config.MaxOutputTokens),
		Providers:             config.Sandrone.Providers,
		TaskRole:              llm.TaskRoleReviewer,
		DataClassification:    classification,
		ExternalCallBudget:    callBudget,
		ForceJSON:             true,
		RequireCompletePrompt: true,
		FailFastOnIncomplete:  false,
		ResponseValidation:    llm.ResponseValidation{Policy: llm.ResponseValidationReviewResult},
	}
	routePolicy := newReviewRoutePolicy(u.deps.Completer.RouteCapacitiesFor(reviewRequest), reviewRequest.MaxOutputTokens)
	policy := u.deps.Completer.PolicyHashInputs(reviewRequest)
	modelPolicyHash, err := hashJSON(policy)
	if err != nil {
		return fail("모델 정책 hash를 만들지 못했습니다", err)
	}
	runCandidate, err := newWorkflowRun(task, runTarget, config, modelPolicyHash, startedAt, snapshotObservedAt)
	if err != nil {
		return fail("리뷰 실행 식별자를 만들지 못했습니다", err)
	}
	run, err := u.deps.Runs.CreateOrGetRun(ctx, runCandidate)
	if err != nil {
		return fail("리뷰 실행을 저장하지 못했습니다", err)
	}
	workflowRunCreated = true
	workflowRunID = run.ID
	progress.SetRunID(run.ID)
	if run.Status.IsTerminal() {
		if err := u.reconcileTerminalProgress(ctx, target, run, progress); err != nil {
			return fmt.Errorf("종료된 리뷰의 진행 코멘트를 정리하지 못했습니다: %w", err)
		}
		u.deps.Logger.Info("이미 종료된 리뷰 실행을 건너뜁니다", "target", target.Reference(), "run", run.ID, "status", run.Status)
		*outcome = reviewOutcomeOf(run.Status)
		return nil
	}
	resumePublishing := run.Status == reviewworkflow.RunStatusPublishing
	publicationClaimed = resumePublishing
	publicationWasResumed = resumePublishing
	if stale {
		detail := "더 최신인 " + request.HeadSHA + " head가 있어 실행을 중단했습니다"
		var actualStatus reviewworkflow.RunStatus
		var finishErr error
		if resumePublishing {
			actualStatus, finishErr = u.supersedePublishedReview(ctx, target, reviewworkflow.PublicationMarker(run.Key), run.ID, "", request.HeadSHA, detail, progress)
		} else {
			actualStatus, finishErr = u.finishWorkflowSupersededByHeadWithProgress(ctx, run.ID, "", request.HeadSHA, detail, progress)
		}
		if finishErr != nil {
			return fail("오래된 리뷰 실행을 종료하지 못했습니다", finishErr)
		}
		if actualStatus != reviewworkflow.RunStatusSuperseded {
			*outcome = reviewOutcomeOf(actualStatus)
			return nil
		}
		if progress != nil {
			if notifyErr := u.notify(ctx, target, task, progress, review.Notice{Kind: review.NoticeSuperseded, Message: "더 최신 변경이 감지되어 이 리뷰를 종료했습니다. 최신 리뷰 실행이 이어서 처리합니다."}); notifyErr != nil {
				return fmt.Errorf("대체된 리뷰의 진행 코멘트를 정리하지 못했습니다: %w", notifyErr)
			}
		}
		u.save(ctx, task, startedAt, review.OutcomeSuperseded, detail, llm.Response{}, 0, 0, run.ID)
		*outcome = reviewOutcomeOf(actualStatus)
		return nil
	}
	reason, skipped := skipReason(task, config, false)
	if skipped && task.Trigger == review.TriggerPullRequestPushed && config.Sandrone.AutoReview {
		continuesRegisteredReview, lookupErr := u.deps.Runs.HasRegisteredReviewContinuation(ctx, run, activityBoundary)
		if lookupErr != nil {
			return fail("등록된 리뷰 실행의 연속성을 확인하지 못했습니다", lookupErr)
		}
		reason, skipped = skipReason(task, config, continuesRegisteredReview)
		if continuesRegisteredReview {
			u.deps.Logger.Info("등록된 리뷰 실행을 현재 head에서 계속합니다", "target", target.Reference(), "run", run.ID, "head", run.HeadSHA)
		}
	}
	if skipped {
		u.deps.Logger.Info("설정에 따라 자동 리뷰를 건너뜁니다", "target", target.Reference(), "reason", reason)
		actualStatus, finishErr := u.finishWorkflowWithProgress(ctx, run.ID, "", reviewworkflow.RunStatusSkipped, reason, false, progress)
		if finishErr != nil {
			return fail("건너뛴 리뷰 실행을 저장하지 못했습니다", finishErr)
		}
		if actualStatus != reviewworkflow.RunStatusSkipped {
			*outcome = reviewOutcomeOf(actualStatus)
			return nil
		}
		if progress != nil {
			if notifyErr := u.notify(ctx, target, task, progress, review.Notice{Kind: review.NoticeSkipped, Message: "리뷰할 변경 사항이 없습니다."}); notifyErr != nil {
				return fmt.Errorf("건너뛴 리뷰의 진행 코멘트를 정리하지 못했습니다: %w", notifyErr)
			}
		}
		u.save(ctx, task, startedAt, review.OutcomeSkipped, reason, llm.Response{}, 0, 0, run.ID)
		*outcome = review.OutcomeSkipped
		return nil
	}
	registrationState, registrationErr := u.deps.Source.PullRequest(ctx, target)
	if registrationErr != nil {
		return fail("리뷰 실행 등록 전 Pull Request 상태를 확인하지 못했습니다", registrationErr)
	}
	registrationTarget, registrationMatches, registrationMatchErr := u.rebindNoopHeadChange(ctx, target, registrationState)
	if registrationMatchErr != nil {
		return fail("리뷰 실행 등록 전 head 이동의 실제 파일 변경을 확인하지 못했습니다", registrationMatchErr)
	}
	if !registrationMatches {
		detail := "리뷰 실행 등록 전에 base 또는 head가 변경되었습니다"
		supersedingHeadSHA := changedHeadSHA(target.HeadSHA, registrationState.HeadSHA)
		var actualStatus reviewworkflow.RunStatus
		var finishErr error
		if resumePublishing {
			actualStatus, finishErr = u.supersedePublishedReview(ctx, target, reviewworkflow.PublicationMarker(run.Key), run.ID, "", supersedingHeadSHA, detail, progress)
		} else {
			actualStatus, finishErr = u.finishWorkflowSupersededByHeadWithProgress(ctx, run.ID, "", supersedingHeadSHA, detail, progress)
		}
		if finishErr != nil {
			return fail("오래된 리뷰 실행을 종료하지 못했습니다", finishErr)
		}
		if actualStatus != reviewworkflow.RunStatusSuperseded {
			*outcome = reviewOutcomeOf(actualStatus)
			return nil
		}
		if progress != nil && !resumePublishing {
			if notifyErr := u.notify(ctx, target, task, progress, review.Notice{Kind: review.NoticeSuperseded, Message: "더 최신 변경이 감지되어 이 리뷰를 종료했습니다. 최신 리뷰 실행이 이어서 처리합니다."}); notifyErr != nil {
				return fmt.Errorf("대체된 리뷰의 진행 코멘트를 정리하지 못했습니다: %w", notifyErr)
			}
		}
		u.save(ctx, task, startedAt, review.OutcomeSuperseded, detail, llm.Response{}, 0, 0, run.ID)
		*outcome = reviewOutcomeOf(actualStatus)
		return nil
	}
	target = registrationTarget
	task.Target = target
	leaseStartedAt := u.deps.Clock.Now()
	acquiredRunLease, leaseErr := u.deps.Runs.AcquireRun(ctx, run.ID, leaseStartedAt, leaseStartedAt.Add(reviewRunLease))
	if errors.Is(leaseErr, reviewworkflow.ErrRunSuperseded) {
		detail := "더 최신인 리뷰 실행이 있어 중단했습니다"
		var actualStatus reviewworkflow.RunStatus
		var finishErr error
		if resumePublishing {
			actualStatus, finishErr = u.supersedePublishedReview(ctx, target, reviewworkflow.PublicationMarker(run.Key), run.ID, "", "", detail, progress)
		} else {
			actualStatus, finishErr = u.finishWorkflowWithProgress(ctx, run.ID, "", reviewworkflow.RunStatusSuperseded, detail, false, progress)
		}
		if finishErr != nil {
			return fail("대체된 리뷰 실행을 종료하지 못했습니다", finishErr)
		}
		if actualStatus != reviewworkflow.RunStatusSuperseded {
			*outcome = reviewOutcomeOf(actualStatus)
			return nil
		}
		if progress != nil && !resumePublishing {
			if notifyErr := u.notify(ctx, target, task, progress, review.Notice{Kind: review.NoticeSuperseded, Message: "더 최신 변경이 감지되어 이 리뷰를 종료했습니다. 최신 리뷰 실행이 이어서 처리합니다."}); notifyErr != nil {
				return fmt.Errorf("대체된 리뷰의 진행 코멘트를 정리하지 못했습니다: %w", notifyErr)
			}
		}
		u.save(ctx, task, startedAt, review.OutcomeSuperseded, detail, llm.Response{}, 0, 0, run.ID)
		*outcome = reviewOutcomeOf(actualStatus)
		return nil
	}
	if errors.Is(leaseErr, reviewworkflow.ErrRunLeased) {
		u.deps.Logger.Info("다른 worker가 같은 리뷰 실행을 처리 중입니다", "target", target.Reference(), "run", run.ID)
		return leaseErr
	}
	if errors.Is(leaseErr, reviewworkflow.ErrPublicationLeased) || errors.Is(leaseErr, reviewworkflow.ErrProgressCommentLeased) {
		u.deps.Logger.Info("다른 게시 또는 진행 코멘트 작업이 리뷰 실행을 막고 있습니다", "target", target.Reference(), "run", run.ID)
		if task.FinalAttempt {
			return fail("최종 시도에서 리뷰 실행 lease를 얻지 못했습니다", leaseErr)
		}
		return leaseErr
	}
	if leaseErr != nil {
		return fail("리뷰 실행 lease를 얻지 못했습니다", leaseErr)
	}
	runLease := acquiredRunLease.Token
	if runLease == "" {
		currentRun, currentRunErr := u.deps.Runs.CreateOrGetRun(ctx, runCandidate)
		if currentRunErr != nil {
			return fail("lease가 없는 리뷰 실행 상태를 다시 읽지 못했습니다", currentRunErr)
		}
		if !currentRun.Status.IsTerminal() {
			return fail("리뷰 실행 lease를 얻지 못했습니다", reviewworkflow.ErrRunLeased)
		}
		if reconcileErr := u.reconcileTerminalProgress(ctx, target, currentRun, progress); reconcileErr != nil {
			return fmt.Errorf("종료된 리뷰의 진행 코멘트를 정리하지 못했습니다: %w", reconcileErr)
		}
		*outcome = reviewOutcomeOf(currentRun.Status)
		return nil
	}
	activeRunID = run.ID
	activeRunLease = runLease
	externalCalls := acquiredRunLease.ExternalCalls
	if externalCalls < 0 {
		externalCalls = 0
	}
	if externalCalls > maxCalls {
		externalCalls = maxCalls
	}
	reserveExternalCall := func(limit int) func() error {
		return func() error {
			heartbeatAt := u.deps.Clock.Now()
			reserved, reserveErr := u.deps.Execution.ReserveExternalCall(ctx, reviewworkflow.ExternalCallReservation{
				RunID:             run.ID,
				RunLeaseToken:     runLease,
				Limit:             limit,
				HeartbeatAt:       heartbeatAt,
				RunLeaseExpiresAt: heartbeatAt.Add(reviewRunLease),
			})
			if reserveErr != nil {
				return reserveErr
			}
			if !reserved {
				return llm.ErrExternalCallBudgetExhausted
			}
			return nil
		}
	}
	callBudget = llm.NewScopedDurableExternalCallBudget(maxCalls, externalCalls, maxCalls, reserveExternalCall(maxCalls))
	reviewRequest.ExternalCallBudget = callBudget
	if resumePublishing {
		claimedAt := u.deps.Clock.Now()
		if claimErr := u.deps.Publications.ClaimPublication(ctx, run.ID, runLease, claimedAt, claimedAt.Add(reviewPublicationLease)); claimErr != nil {
			return fail("기존 리뷰 게시 권한을 복구하지 못했습니다", claimErr)
		}
		marker := reviewworkflow.PublicationMarker(run.Key)
		storedPublication, storedPublicationFound, storedPublicationErr := u.deps.Publications.ReviewPublication(ctx, run.ID, runLease)
		if storedPublicationErr != nil {
			return fail("저장된 리뷰 게시 payload를 읽지 못했습니다", storedPublicationErr)
		}
		if storedPublicationFound && storedPublication.Marker != marker {
			return fail("저장된 리뷰 게시 marker가 실행과 일치하지 않습니다", fmt.Errorf("review run %d", run.ID))
		}
		if storedPublicationFound {
			publicationEffectsStarted = true
			_, historyFound, historyErr := u.deps.Reviews.ByRunID(ctx, run.ID)
			if historyErr != nil {
				return fail("기존 리뷰 이력을 확인하지 못했습니다", historyErr)
			}
			if !historyFound {
				return fail("게시 상태에 대응하는 내부 리뷰 이력을 찾지 못했습니다", fmt.Errorf("review run %d", run.ID))
			}
			if storedPublication.PayloadHash != "" {
				publishErr := leaseheartbeat.Run(ctx, reviewPublicationLease, func(heartbeatContext context.Context) error {
					return u.renewReviewPublicationRun(heartbeatContext, run.ID, runLease)
				}, func(publicationContext context.Context) error {
					return u.publishPreparedReview(publicationContext, target, run.ID, runLease, storedPublication)
				})
				if errors.Is(publishErr, publication.ErrTargetChanged) || errors.Is(publishErr, reviewworkflow.ErrRunSuperseded) {
					detail := "저장된 리뷰 게시 payload 재개 직전에 base 또는 head가 변경되었습니다"
					supersedingHeadSHA := u.resolveSupersedingHeadSHA(ctx, target)
					actualStatus, finishErr := u.supersedePublishedReview(ctx, target, marker, run.ID, runLease, supersedingHeadSHA, detail, progress)
					if finishErr != nil {
						return fail("재개 중 대체된 리뷰 실행을 종료하지 못했습니다", finishErr)
					}
					*outcome = reviewOutcomeOf(actualStatus)
					return nil
				} else if publishErr != nil {
					return fail("저장된 리뷰 게시 payload를 재게시하지 못했습니다", publishErr)
				}
				publicationCompleted = true
			}
			finalization := reviewworkflow.ReviewPublicationFinalization{
				Status:           reviewworkflow.RunStatusPartial,
				Detail:           "legacy marker와 게시 receipt만 확인되어 보수적으로 부분 완료했습니다",
				AdvanceWatermark: false,
			}
			if storedPublication.PayloadHash != "" {
				finalization = storedPublication.Finalization
			}
			actualStatus, finishErr := u.finalizePublishedReview(ctx, target, run.ID, runLease, marker, finalization.Status, finalization.Detail, finalization.AdvanceWatermark)
			if finishErr != nil {
				return fail("재개한 리뷰 게시 결과를 완료하지 못했습니다", finishErr)
			}
			*outcome = reviewOutcomeOf(actualStatus)
			return nil
		}
		published := false
		publishedErr := leaseheartbeat.Run(ctx, reviewRunLease, func(heartbeatContext context.Context) error {
			return u.renewReviewRun(heartbeatContext, run.ID, runLease)
		}, func(reconciliationContext context.Context) error {
			var reconciliationErr error
			published, reconciliationErr = u.deps.Publisher.PublicationExists(reconciliationContext, target, marker)
			return reconciliationErr
		})
		if publishedErr != nil {
			return fail("기존 리뷰 게시 결과를 확인하지 못했습니다", publishedErr)
		}
		if published {
			publicationEffectsStarted = true
			_, historyFound, historyErr := u.deps.Reviews.ByRunID(ctx, run.ID)
			if historyErr != nil {
				return fail("기존 리뷰 이력을 확인하지 못했습니다", historyErr)
			}
			if !historyFound {
				return fail("게시 상태에 대응하는 내부 리뷰 이력을 찾지 못했습니다", fmt.Errorf("review run %d", run.ID))
			}
			expiresAt := run.StartedAt.Add(u.deps.Retention)
			if completeErr := u.completeReviewPublication(ctx, run.ID, runLease, marker, reviewworkflow.ReviewPublicationChannelReconciled, 0, expiresAt); completeErr != nil {
				return fail("기존 리뷰 게시 receipt를 완료하지 못했습니다", completeErr)
			}
			finalization := reviewworkflow.ReviewPublicationFinalization{
				Status:           reviewworkflow.RunStatusPartial,
				Detail:           "legacy marker와 게시 receipt만 확인되어 보수적으로 부분 완료했습니다",
				AdvanceWatermark: false,
			}
			actualStatus, finishErr := u.finalizePublishedReview(ctx, target, run.ID, runLease, marker, finalization.Status, finalization.Detail, finalization.AdvanceWatermark)
			if finishErr != nil {
				return fail("재개한 리뷰 게시 결과를 완료하지 못했습니다", finishErr)
			}
			*outcome = reviewOutcomeOf(actualStatus)
			return nil
		}
		if resumeErr := u.deps.Runs.ResumeRun(ctx, run.ID, runLease, u.deps.Clock.Now()); resumeErr != nil {
			return fail("미게시 리뷰 실행을 다시 시작하지 못했습니다", resumeErr)
		}
		publicationClaimed = false
		publicationWasResumed = false
	}
	u.acknowledge(ctx, target, task)
	progress = u.startProgress(run, progress)
	defer progress.Stop()

	var files []pullrequest.ChangedFile
	var incremental bool
	var expectedFiles int
	err = leaseheartbeat.Run(ctx, reviewRunLease, func(heartbeatContext context.Context) error {
		return u.renewReviewRun(heartbeatContext, run.ID, runLease)
	}, func(collectionContext context.Context) error {
		var collectErr error
		files, incremental, expectedFiles, collectErr = u.collectFiles(collectionContext, target, request, task)
		return collectErr
	})
	if err != nil {
		return fail("변경 파일을 읽지 못했습니다", err)
	}
	if err := u.revalidateFindingOccurrences(ctx, target, files, run.ID, runLease); err != nil {
		return fail("기존 지적을 재검증하지 못했습니다", err)
	}
	chosen := selection.FileSelector{
		Include:  config.Include,
		Exclude:  config.Exclude,
		MaxFiles: config.MaxFiles,
	}.Select(files)
	if chosen.IsEmpty() {
		_, coverage := (reviewworkflow.PlanBuilder{MaxFileChars: config.MaxFileChars, ExpectedFiles: expectedFiles}).Build(files, chosen, batching.Plan{})
		if _, planErr := u.deps.Execution.SavePlan(ctx, run.ID, runLease, nil, coverage, u.deps.Clock.Now()); planErr != nil {
			return fail("빈 리뷰 계획을 저장하지 못했습니다", planErr)
		}
		summary := reviewworkflow.SummarizeCoverage(coverage)
		status := summary.TerminalStatus()
		detail := coverageDetail(summary)
		advance := status == reviewworkflow.RunStatusSkipped
		actualStatus, finishErr := u.finishWorkflowWithProgress(ctx, run.ID, runLease, status, detail, advance, progress)
		if finishErr != nil {
			return fail("리뷰 실행을 종료하지 못했습니다", finishErr)
		}
		if actualStatus != status {
			*outcome = reviewOutcomeOf(actualStatus)
			return nil
		}
		if actualStatus == reviewworkflow.RunStatusSuperseded {
			if notifyErr := u.notify(ctx, target, task, progress, review.Notice{Kind: review.NoticeSuperseded, Message: "더 최신 변경이 감지되어 이 리뷰를 종료했습니다. 최신 리뷰 실행이 이어서 처리합니다."}); notifyErr != nil {
				return fmt.Errorf("대체된 리뷰의 진행 코멘트를 정리하지 못했습니다: %w", notifyErr)
			}
			u.save(ctx, task, startedAt, review.OutcomeSuperseded, detail, llm.Response{}, 0, 0, run.ID)
			*outcome = review.OutcomeSuperseded
			return nil
		}
		if actualStatus == reviewworkflow.RunStatusSkipped {
			if notifyErr := u.notify(ctx, target, task, progress, review.Notice{Kind: review.NoticeSkipped, Message: "리뷰할 변경 사항이 없습니다."}); notifyErr != nil {
				return fmt.Errorf("건너뛴 리뷰의 진행 코멘트를 정리하지 못했습니다: %w", notifyErr)
			}
			u.save(ctx, task, startedAt, review.OutcomeSkipped, detail, llm.Response{}, 0, 0, run.ID)
			*outcome = review.OutcomeSkipped
			return nil
		}
		if notifyErr := u.notify(ctx, target, task, progress, review.Notice{Kind: review.NoticeUnavailable, Message: "변경 diff를 확보하지 못해 리뷰를 완료하지 못했습니다."}); notifyErr != nil {
			return fmt.Errorf("종료된 리뷰의 진행 코멘트를 정리하지 못했습니다: %w", notifyErr)
		}
		u.save(ctx, task, startedAt, review.OutcomeUnavailable, detail, llm.Response{}, 0, 0, run.ID)
		*outcome = reviewOutcomeOf(actualStatus)
		return nil
	}
	loaded, err := u.loadSources(ctx, target, chosen.Files, config, run.ID, runLease)
	if err != nil {
		return fail("리뷰 source를 수집하는 동안 실행 lease를 갱신하지 못했습니다", err)
	}
	pathMap := newPromptPathMap(files, u.deps.Masker.Mask)

	if err := u.renewReviewRun(ctx, run.ID, runLease); err != nil {
		return fail("리뷰 지침 수집 전 실행 lease를 갱신하지 못했습니다", err)
	}
	instructions := instruction.Collection{}
	err = leaseheartbeat.Run(ctx, reviewRunLease, func(heartbeatContext context.Context) error {
		return u.renewReviewRun(heartbeatContext, run.ID, runLease)
	}, func(collectionContext context.Context) error {
		var instructionErr error
		instructions, instructionErr = u.deps.Settings.Instructions(collectionContext, target, config)
		return instructionErr
	})
	if err != nil {
		return fail("리뷰 지침 문서를 읽지 못했습니다", err)
	}
	if err := u.renewReviewRun(ctx, run.ID, runLease); err != nil {
		return fail("리뷰 지침 수집 후 실행 lease를 갱신하지 못했습니다", err)
	}

	inventory := inventoryOf(files)
	toolsAllowed := config.MaxExtraReads > 0 && u.deps.Tools != nil
	planner := reviewBatchPlanner{
		PullRequest:       request,
		Inventory:         inventory,
		TotalChangedFiles: request.ChangedFiles,
		Instructions:      instructions,
		Config:            config,
		Incremental:       incremental,
		Extra:             task.Instruction,
		ToolsAllowed:      toolsAllowed,
		ProviderLimit:     routePolicy.promptLimit,
		MaxReviewBatches:  config.Sandrone.MaxReviewBatches,
		RoutePolicy:       routePolicy,
		Mask:              u.deps.Masker.Mask,
		Paths:             pathMap,
	}
	includeFileNotes := planner.includeFileNotes()
	plan, promptConfig := planner.Build(loaded)
	units, coverage := (reviewworkflow.PlanBuilder{MaxFileChars: config.MaxFileChars, ExpectedFiles: expectedFiles}).Build(files, chosen, plan)
	unitExecution, unitExecutionErr := u.unitExecutor.execute(ctx, reviewUnitExecution{
		target:            target,
		runID:             run.ID,
		runConfigHash:     run.ConfigHash,
		runLease:          runLease,
		config:            config,
		planner:           planner,
		plan:              plan,
		promptConfig:      promptConfig,
		units:             units,
		coverage:          coverage,
		request:           reviewRequest,
		paths:             pathMap,
		includeFileNotes:  includeFileNotes,
		maxCalls:          maxCalls,
		reviewerCallLimit: reviewerCallLimit,
		externalCalls:     externalCalls,
	})
	failureResponse = unitExecution.response
	if unitExecutionErr != nil {
		return fail(unitExecution.errorMessage, unitExecutionErr)
	}
	response := unitExecution.response
	reviewed := unitExecution.reviewed
	failed := unitExecution.failed
	if len(failed) > 0 && !task.FinalAttempt && !unitExecution.budgetExhausted && unitExecution.retryable {
		retryAt := unitExecution.retryAt
		if retryAt.IsZero() {
			retryAt = u.deps.Clock.Now().Add(time.Minute)
		}
		cause := fmt.Errorf("%d개 파일의 일시 실패가 남아 있습니다", len(failed))
		u.deps.Logger.Info("일시 실패한 리뷰 unit을 사용자 실패로 처리하지 않고 재개를 예약합니다", "target", target.Reference(), "run", run.ID, "files", len(failed), "retry_at", retryAt)
		return &job.RetryAtError{At: retryAt, Cause: cause}
	}
	coverageSummary := reviewworkflow.SummarizeCoverage(coverage)
	runStatus := coverageSummary.TerminalStatus()
	if len(reviewed) == 0 || runStatus == reviewworkflow.RunStatusFailed {
		detail := coverageDetail(coverageSummary)
		projectionOutcome := reviewOutcomeOf(runStatus)
		if runStatus == reviewworkflow.RunStatusFailed && len(failed) > 0 {
			projectionOutcome = review.OutcomeFailed
		}
		actualStatus, finishErr := u.finishWorkflowWithOutcomeAndProgress(ctx, run.ID, runLease, runStatus, projectionOutcome, detail, false, progress)
		if finishErr != nil {
			return fail("실패한 리뷰 실행을 종료하지 못했습니다", finishErr)
		}
		if actualStatus != runStatus {
			*outcome = reviewOutcomeOf(actualStatus)
			return nil
		}
		if actualStatus == reviewworkflow.RunStatusSuperseded {
			if notifyErr := u.notify(ctx, target, task, progress, review.Notice{Kind: review.NoticeSuperseded, Message: "더 최신 변경이 감지되어 이 리뷰를 종료했습니다. 최신 리뷰 실행이 이어서 처리합니다."}); notifyErr != nil {
				return fmt.Errorf("대체된 리뷰의 진행 코멘트를 정리하지 못했습니다: %w", notifyErr)
			}
			u.save(ctx, task, startedAt, review.OutcomeSuperseded, detail, response, 0, 0, run.ID)
			*outcome = review.OutcomeSuperseded
			return nil
		}
		if len(failed) == 0 {
			if notifyErr := u.notify(ctx, target, task, progress, review.Notice{Kind: review.NoticeUnavailable, Message: "검토 가능한 전체 diff를 확보하지 못해 리뷰를 완료하지 못했습니다."}); notifyErr != nil {
				return fmt.Errorf("종료된 리뷰의 진행 코멘트를 정리하지 못했습니다: %w", notifyErr)
			}
			u.save(ctx, task, startedAt, review.OutcomeUnavailable, detail, response, 0, 0, run.ID)
			*outcome = reviewOutcomeOf(actualStatus)
			return nil
		}
		if unitExecution.budgetExhausted {
			if notifyErr := u.notify(ctx, target, task, progress, review.Notice{Kind: review.NoticeUnavailable, Message: "정해진 호출 예산 안에서 유효한 리뷰 결과를 만들지 못했습니다."}); notifyErr != nil {
				return fmt.Errorf("종료된 리뷰의 진행 코멘트를 정리하지 못했습니다: %w", notifyErr)
			}
			u.save(ctx, task, startedAt, review.OutcomeFailed, detail, response, 0, 0, run.ID)
			*outcome = review.OutcomeFailed
			return nil
		}
		if notifyErr := u.notify(ctx, target, task, progress, review.Notice{Kind: review.NoticeFailed, Message: "모든 리뷰 프로바이더를 시도했지만 유효한 리뷰 결과를 만들지 못해 이번 리뷰를 종료합니다."}); notifyErr != nil {
			return fmt.Errorf("실패한 리뷰의 진행 코멘트를 정리하지 못했습니다: %w", notifyErr)
		}
		u.save(ctx, task, startedAt, review.OutcomeFailed, detail, response, 0, 0, run.ID)
		*outcome = review.OutcomeFailed
		return nil
	}

	verifierCallLimit := unitExecution.externalCalls + verifierCallReserve
	if verifierCallLimit > maxCalls {
		verifierCallLimit = maxCalls
	}
	reviewRequest.ExternalCallBudget = llm.NewScopedDurableExternalCallBudget(maxCalls, unitExecution.externalCalls, verifierCallLimit, reserveExternalCall(verifierCallLimit))
	finalizedResult, finalizationErr := u.resultFinalizer.finalize(ctx, reviewResultFinalization{
		target:      target,
		runID:       run.ID,
		runLease:    runLease,
		config:      config,
		execution:   unitExecution,
		request:     reviewRequest,
		paths:       pathMap,
		trigger:     task.Trigger,
		incremental: incremental,
		runStatus:   runStatus,
	})
	failureResponse = finalizedResult.failureResponse
	if finalizationErr != nil {
		return fail(finalizedResult.errorMessage, finalizationErr)
	}
	placed := finalizedResult.draft
	view := finalizedResult.view
	attribution := finalizedResult.attribution
	style := finalizedResult.style
	response = finalizedResult.response
	runStatus = finalizedResult.runStatus
	verificationUnavailable := finalizedResult.verificationUnavailable
	heartbeatAt := u.deps.Clock.Now()
	if leaseErr := u.deps.Runs.RenewRun(ctx, run.ID, runLease, heartbeatAt, heartbeatAt.Add(reviewRunLease)); leaseErr != nil {
		if errors.Is(leaseErr, reviewworkflow.ErrRunSuperseded) {
			detail := "게시 전에 더 최신인 리뷰 실행이 확인되었습니다"
			actualStatus, finishErr := u.finishWorkflowWithProgress(ctx, run.ID, runLease, reviewworkflow.RunStatusSuperseded, detail, false, progress)
			if finishErr != nil {
				return fail("대체된 리뷰 실행을 종료하지 못했습니다", finishErr)
			}
			if actualStatus != reviewworkflow.RunStatusSuperseded {
				*outcome = reviewOutcomeOf(actualStatus)
				return nil
			}
			u.save(ctx, task, startedAt, review.OutcomeSuperseded, detail, response, 0, 0, run.ID)
			if notifyErr := u.notify(ctx, target, task, progress, review.Notice{Kind: review.NoticeSuperseded, Message: "더 최신 변경이 감지되어 이 리뷰를 종료했습니다. 최신 리뷰 실행이 이어서 처리합니다."}); notifyErr != nil {
				return fmt.Errorf("대체된 리뷰의 진행 코멘트를 정리하지 못했습니다: %w", notifyErr)
			}
			*outcome = review.OutcomeSuperseded
			return nil
		}
		return fail("게시 전 리뷰 실행 lease를 갱신하지 못했습니다", leaseErr)
	}
	latest, latestErr := u.deps.Source.PullRequest(ctx, target)
	if latestErr != nil {
		return fail("게시 전 Pull Request 상태를 확인하지 못했습니다", latestErr)
	}
	latestTarget, latestMatches, latestMatchErr := u.rebindNoopHeadChange(ctx, target, latest)
	if latestMatchErr != nil {
		return fail("게시 전 head 이동의 실제 파일 변경을 확인하지 못했습니다", latestMatchErr)
	}
	if !latestMatches {
		detail := "게시 전에 base 또는 head가 변경되었습니다"
		actualStatus, finishErr := u.finishWorkflowSupersededByHeadWithProgress(ctx, run.ID, runLease, changedHeadSHA(target.HeadSHA, latest.HeadSHA), detail, progress)
		if finishErr != nil {
			return fail("오래된 리뷰 실행을 종료하지 못했습니다", finishErr)
		}
		if actualStatus != reviewworkflow.RunStatusSuperseded {
			*outcome = reviewOutcomeOf(actualStatus)
			return nil
		}
		u.save(ctx, task, startedAt, review.OutcomeSuperseded, detail, response, 0, 0, run.ID)
		if notifyErr := u.notify(ctx, target, task, progress, review.Notice{Kind: review.NoticeSuperseded, Message: "더 최신 변경이 감지되어 이 리뷰를 종료했습니다. 최신 리뷰 실행이 이어서 처리합니다."}); notifyErr != nil {
			return fmt.Errorf("대체된 리뷰의 진행 코멘트를 정리하지 못했습니다: %w", notifyErr)
		}
		*outcome = review.OutcomeSuperseded
		return nil
	}
	target = latestTarget
	task.Target = target

	publicationClaimedAt := u.deps.Clock.Now()
	if claimErr := u.deps.Publications.ClaimPublication(ctx, run.ID, runLease, publicationClaimedAt, publicationClaimedAt.Add(reviewPublicationLease)); claimErr != nil {
		if errors.Is(claimErr, reviewworkflow.ErrRunSuperseded) {
			detail := "게시 권한을 얻기 전에 더 최신인 리뷰 실행이 확인되었습니다"
			actualStatus, finishErr := u.finishWorkflowWithProgress(ctx, run.ID, runLease, reviewworkflow.RunStatusSuperseded, detail, false, progress)
			if finishErr != nil {
				return fail("대체된 리뷰 실행을 종료하지 못했습니다", finishErr)
			}
			if actualStatus != reviewworkflow.RunStatusSuperseded {
				*outcome = reviewOutcomeOf(actualStatus)
				return nil
			}
			u.save(ctx, task, startedAt, review.OutcomeSuperseded, detail, response, 0, 0, run.ID)
			if notifyErr := u.notify(ctx, target, task, progress, review.Notice{Kind: review.NoticeSuperseded, Message: "더 최신 변경이 감지되어 이 리뷰를 종료했습니다. 최신 리뷰 실행이 이어서 처리합니다."}); notifyErr != nil {
				return fmt.Errorf("대체된 리뷰의 진행 코멘트를 정리하지 못했습니다: %w", notifyErr)
			}
			*outcome = review.OutcomeSuperseded
			return nil
		}
		return fail("리뷰 게시 권한을 얻지 못했습니다", claimErr)
	}
	publicationClaimed = true
	confirmed, confirmedErr := u.deps.Source.PullRequest(ctx, target)
	if confirmedErr != nil {
		return fail("게시 직전 Pull Request 상태를 확인하지 못했습니다", confirmedErr)
	}
	confirmedTarget, confirmedMatches, confirmedMatchErr := u.rebindNoopHeadChange(ctx, target, confirmed)
	if confirmedMatchErr != nil {
		return fail("게시 직전 head 이동의 실제 파일 변경을 확인하지 못했습니다", confirmedMatchErr)
	}
	if !confirmedMatches {
		detail := "게시 권한을 얻은 뒤 base 또는 head가 변경되었습니다"
		actualStatus, finishErr := u.finishWorkflowSupersededByHeadWithProgress(ctx, run.ID, runLease, changedHeadSHA(target.HeadSHA, confirmed.HeadSHA), detail, progress)
		if finishErr != nil {
			return fail("오래된 리뷰 실행을 종료하지 못했습니다", finishErr)
		}
		if actualStatus != reviewworkflow.RunStatusSuperseded {
			*outcome = reviewOutcomeOf(actualStatus)
			return nil
		}
		u.save(ctx, task, startedAt, review.OutcomeSuperseded, detail, response, 0, 0, run.ID)
		if notifyErr := u.notify(ctx, target, task, progress, review.Notice{Kind: review.NoticeSuperseded, Message: "더 최신 변경이 감지되어 이 리뷰를 종료했습니다. 최신 리뷰 실행이 이어서 처리합니다."}); notifyErr != nil {
			return fmt.Errorf("대체된 리뷰의 진행 코멘트를 정리하지 못했습니다: %w", notifyErr)
		}
		*outcome = review.OutcomeSuperseded
		return nil
	}
	target = confirmedTarget
	task.Target = target

	provisionalDetail := "GitHub 게시 결과 확인 대기 중"
	if _, persistErr := u.saveWithFindings(ctx, task, startedAt, review.OutcomeUnavailable, provisionalDetail, response, len(placed.Inline()), len(placed.Fallback()), run.ID, run.StartedAt.Add(u.deps.Retention), placed.Findings); persistErr != nil {
		return fail("게시 전 리뷰 이력과 지적을 저장하지 못했습니다", persistErr)
	}
	publicationMarker := reviewworkflow.PublicationMarker(run.Key)
	detail := coverageDetail(coverageSummary)
	if verificationUnavailable {
		detail += " verification=unavailable"
	}
	finalization := reviewworkflow.ReviewPublicationFinalization{
		Status:           runStatus,
		Detail:           detail,
		AdvanceWatermark: runStatus == reviewworkflow.RunStatusComplete,
	}
	publishErr := leaseheartbeat.Run(ctx, reviewPublicationLease, func(heartbeatContext context.Context) error {
		return u.renewReviewPublicationRun(heartbeatContext, run.ID, runLease)
	}, func(publicationContext context.Context) error {
		return u.prepareAndPublishReview(publicationContext, target, run.ID, runLease, publicationMarker, progress.Markers(), view, placed, attribution, style, finalization, func(preparedContext context.Context) error {
			progress.Stop()
			publicationEffectsStarted = true
			return u.waitForProgressMutationFence(preparedContext, target, progress)
		})
	})
	if errors.Is(publishErr, publication.ErrTargetChanged) || errors.Is(publishErr, reviewworkflow.ErrRunSuperseded) {
		progress.Stop()
		detail := "리뷰 게시 요청 직전에 base 또는 head가 변경되었습니다"
		supersedingHeadSHA := u.resolveSupersedingHeadSHA(ctx, target)
		actualStatus, finishErr := u.supersedePublishedReview(ctx, target, publicationMarker, run.ID, runLease, supersedingHeadSHA, detail, progress)
		if finishErr != nil {
			return fail("게시 직전에 대체된 리뷰 실행을 종료하지 못했습니다", finishErr)
		}
		u.save(ctx, task, startedAt, review.OutcomeSuperseded, detail, response, 0, 0, run.ID)
		*outcome = reviewOutcomeOf(actualStatus)
		return nil
	} else if publishErr != nil {
		if !publicationEffectsStarted {
			resumeContext, resumeCancel := context.WithTimeout(context.WithoutCancel(ctx), reactionTimeout)
			resumeErr := u.deps.Runs.ResumeRun(resumeContext, run.ID, runLease, u.deps.Clock.Now())
			resumeCancel()
			if resumeErr != nil {
				return fail("리뷰 게시 준비 실패 후 실행 상태를 복구하지 못했습니다", errors.Join(publishErr, resumeErr))
			}
			publicationClaimed = false
		}
		return fail("리뷰 게시 결과를 확인하지 못했습니다", publishErr)
	}
	publicationCompleted = true
	actualStatus, finishErr := u.finalizePublishedReview(ctx, target, run.ID, runLease, publicationMarker, finalization.Status, finalization.Detail, finalization.AdvanceWatermark)
	if finishErr != nil {
		return fail("리뷰 실행을 완료하지 못했습니다", finishErr)
	}
	*outcome = reviewOutcomeOf(actualStatus)
	return nil
}

func (u *UseCase) acknowledge(ctx context.Context, target pullrequest.Target, task job.ReviewJob) {
	if !task.Trigger.IsAutomatic() || task.ReactionAdded {
		return
	}
	reactionContext, cancel := context.WithTimeout(ctx, reactionTimeout)
	defer cancel()
	if err := u.deps.Reactions.AddPullRequestReaction(reactionContext, target, "eyes"); err != nil {
		u.deps.Logger.Warn("Pull Request에 리액션을 남기지 못했습니다", "target", target.Reference(), "error", err)
	}
}

func (u *UseCase) collectFiles(ctx context.Context, target pullrequest.Target, request pullrequest.PullRequest, task job.ReviewJob) ([]pullrequest.ChangedFile, bool, int, error) {
	if task.Incremental && u.deps.State != nil {
		previousSHA, stateErr := u.deps.State.LastReviewedSHA(ctx, target, u.deps.Clock.Now().Add(-u.deps.Retention))
		if stateErr != nil {
			u.deps.Logger.Warn("증분 리뷰 기준점을 읽지 못해 전체 변경을 검토합니다", "target", target.Reference(), "error", stateErr)
		} else if previousSHA == request.HeadSHA {
			return nil, true, 0, nil
		} else if previousSHA != "" {
			incrementalFiles, incrementalErr := u.deps.Source.ChangedFilesBetween(ctx, target, previousSHA, request.HeadSHA)
			if incrementalErr == nil {
				return incrementalFiles, true, len(incrementalFiles), nil
			}
			u.deps.Logger.Warn("증분 diff를 읽지 못해 전체 변경을 검토합니다", "target", target.Reference(), "base", previousSHA, "error", incrementalErr)
		}
	}
	files, err := u.deps.Source.ChangedFilesBetween(ctx, target, request.BaseSHA, request.HeadSHA)
	if err != nil || len(files) == 0 || request.ChangedFiles > len(files) {
		fallback, listErr := u.deps.Source.ChangedFiles(ctx, target)
		return fallback, false, request.ChangedFiles, listErr
	}
	return files, false, request.ChangedFiles, nil
}

func (u *UseCase) loadSources(ctx context.Context, target pullrequest.Target, files []pullrequest.ChangedFile, config setting.RepoConfig, runID uint64, runLease string) ([]pullrequest.ChangedFile, error) {
	limit := config.MaxSourceChars
	if config.MaxFileChars > 0 && config.MaxFileChars < limit {
		limit = config.MaxFileChars
	}
	loaded := make([]pullrequest.ChangedFile, 0, len(files))
	for _, file := range files {
		file.Patch = u.deps.Masker.Mask(file.Patch)
		if config.MaxFileChars > 0 && len(file.Patch) > config.MaxFileChars {
			file.Patch = truncateUTF8(file.Patch, config.MaxFileChars)
			file.PatchTruncated = true
		}
		if config.IncludeSources && limit > 0 && !file.IsRemoved() {
			content, err := u.deps.Source.FileContent(ctx, target, file.Path, target.HeadSHA)
			if renewErr := u.renewReviewRun(ctx, runID, runLease); renewErr != nil {
				return nil, renewErr
			}
			if err != nil {
				if errors.Is(err, outbound.ErrRepositoryContentNotFound) || errors.Is(err, outbound.ErrRepositoryContentUnavailable) {
					u.deps.Logger.Info("전체 source를 사용할 수 없어 diff만 리뷰합니다", "target", target.Reference(), "path", u.deps.Masker.Mask(file.Path))
					loaded = append(loaded, file)
					continue
				}
				return nil, fmt.Errorf("%s source를 읽지 못했습니다: %w", u.deps.Masker.Mask(file.Path), err)
			}
			content = u.deps.Masker.Mask(content)
			if len(content) > limit {
				content = truncateUTF8(content, limit)
				file.Truncated = true
			}
			file.Content = content
		}
		loaded = append(loaded, file)
	}
	return loaded, nil
}

func (u *UseCase) renewReviewRun(ctx context.Context, runID uint64, runLease string) error {
	heartbeatAt := u.deps.Clock.Now()
	return u.deps.Runs.RenewRun(ctx, runID, runLease, heartbeatAt, heartbeatAt.Add(reviewRunLease))
}

func (u *UseCase) renewReviewPublicationRun(ctx context.Context, runID uint64, runLease string) error {
	heartbeatAt := u.deps.Clock.Now()
	return u.deps.Runs.RenewRun(ctx, runID, runLease, heartbeatAt, heartbeatAt.Add(reviewPublicationLease))
}

func truncateUTF8(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	for limit > 0 && !utf8.RuneStart(value[limit]) {
		limit--
	}
	return value[:limit]
}

func invalidatedReviewBody(detail string) string {
	return "⚠️ 이 리뷰는 게시 과정에서 더 최신인 실행 또는 PR 기준점이 확인되어 무효화되었습니다.\n\n" + detail
}

func (u *UseCase) supersedePublishedReview(ctx context.Context, target pullrequest.Target, marker string, runID uint64, leaseToken string, supersedingHeadSHA string, detail string, progress *progressSession) (reviewworkflow.RunStatus, error) {
	body := invalidatedReviewBody(detail)
	if progress != nil {
		progress.Stop()
		fenceContext, fenceCancel := context.WithTimeout(context.WithoutCancel(ctx), reviewworkflow.PublicationInvalidationFenceDelay+progressUpdateTimeout)
		fenceErr := u.waitForProgressMutationFence(fenceContext, target, progress)
		fenceCancel()
		if fenceErr != nil {
			return reviewworkflow.RunStatusPublishing, fenceErr
		}
	}
	invalidation := &reviewworkflow.PublicationInvalidation{
		Marker:        marker,
		Reason:        body,
		LastError:     "게시 무효화 최종 확인 대기 중",
		NextAttemptAt: u.deps.Clock.Now().Add(reviewworkflow.PublicationInvalidationFenceDelay),
	}
	actualStatus, finishErr := u.finishWorkflowSupersededByHeadWithInvalidation(ctx, runID, leaseToken, supersedingHeadSHA, detail, invalidation)
	if finishErr != nil {
		return reviewworkflow.RunStatusPublishing, finishErr
	}
	if actualStatus != reviewworkflow.RunStatusSuperseded {
		return actualStatus, nil
	}
	if cleanupErr := u.deps.Publisher.InvalidateReview(ctx, target, marker, body); cleanupErr != nil {
		u.deps.Logger.Warn("게시된 리뷰를 즉시 무효화하지 못해 재조정을 기다립니다", "target", target.Reference(), "error", cleanupErr)
	}
	return actualStatus, nil
}

func (u *UseCase) resolveSupersedingHeadSHA(ctx context.Context, target pullrequest.Target) string {
	current, err := u.deps.Source.PullRequest(ctx, target)
	if err != nil {
		u.deps.Logger.Warn("대체 head를 확인하지 못했습니다", "target", target.Reference(), "error", err)
		return ""
	}
	return changedHeadSHA(target.HeadSHA, current.HeadSHA)
}

func (u *UseCase) notify(ctx context.Context, target pullrequest.Target, task job.ReviewJob, progress *progressSession, notice review.Notice) error {
	body := u.deps.Renderer.NoticeBody(notice)
	replaced, replaceErr := u.replaceProgress(ctx, target, progress, body, notice.Message, !task.Trigger.IsAutomatic())
	if replaceErr != nil {
		return replaceErr
	}
	if replaced {
		return nil
	}
	if task.Trigger.IsAutomatic() {
		return nil
	}
	body = progressResultBody(progress, body)
	if err := u.createNoticeComment(ctx, target, task, progress, body); err != nil {
		return fmt.Errorf("안내 코멘트를 남기지 못했습니다: %w", err)
	}
	return nil
}

func (u *UseCase) fail(ctx context.Context, task job.ReviewJob, startedAt time.Time, message string, cause error, runID uint64, recordFailure bool, response llm.Response, progress *progressSession) error {
	u.deps.Logger.Error(message, "target", task.Target.Reference(), "error", cause)
	if notice, announce := failureNotice(task.Attempt, task.FinalAttempt); announce && (task.FinalAttempt || runID != 0) {
		if err := u.announce(ctx, task.Target, task, progress, notice); err != nil {
			u.deps.Logger.Warn("실패 안내를 남기지 못했습니다", "target", task.Target.Reference(), "error", err)
		}
	}
	if task.FinalAttempt && recordFailure {
		u.save(ctx, task, startedAt, review.OutcomeFailed, message, response, 0, 0, runID)
	}
	return fmt.Errorf("%s: %w", message, cause)
}

func (u *UseCase) announce(ctx context.Context, target pullrequest.Target, task job.ReviewJob, progress *progressSession, notice review.Notice) error {
	body := u.deps.Renderer.NoticeBody(notice)
	replaced, replaceErr := u.replaceProgress(ctx, target, progress, body, notice.Message, true)
	if replaceErr != nil {
		return replaceErr
	}
	if replaced {
		return nil
	}
	body = progressResultBody(progress, body)
	return u.createNoticeComment(ctx, target, task, progress, body)
}

func (u *UseCase) createNoticeComment(ctx context.Context, target pullrequest.Target, task job.ReviewJob, progress *progressSession, body string) error {
	marker := reviewNoticePublicationMarker(task, target)
	_, exists, lookupErr := u.deps.Publisher.FindComment(ctx, target, marker)
	if lookupErr != nil || exists {
		return lookupErr
	}
	body = strings.TrimRight(body, "\n") + "\n" + marker
	if _, createErr := u.deps.Publisher.CreateComment(ctx, target, body); createErr != nil {
		if progress != nil {
			u.markProgressMutationUncertain(progress, createErr)
		}
		_, reconciled, reconcileErr := u.deps.Publisher.FindComment(ctx, target, marker)
		if reconciled {
			return nil
		}
		return errors.Join(createErr, reconcileErr)
	}
	return nil
}

func reviewNoticePublicationMarker(task job.ReviewJob, target pullrequest.Target) string {
	identity := workflowRequestIdentity(task)
	if identity == "" {
		identity = strings.Join([]string{
			string(task.Trigger),
			task.Target.BaseSHA,
			task.Target.HeadSHA,
			task.RequestReceivedAt.UTC().Format(time.RFC3339Nano),
			task.SnapshotObservedAt.UTC().Format(time.RFC3339Nano),
			task.SnapshotOrderKey,
		}, "\x00")
	}
	return job.PublicationMarker("review-final-notice", target, identity, task.CommentID, task.InThread)
}

func failureNotice(_ int, final bool) (review.Notice, bool) {
	if final {
		return review.Notice{Kind: review.NoticeFailed, Message: "리뷰를 완료하지 못했습니다. 재시도했지만 해결되지 않아 중단합니다."}, true
	}
	return review.Notice{}, false
}

func (u *UseCase) save(ctx context.Context, task job.ReviewJob, startedAt time.Time, outcome review.Outcome, detail string, response llm.Response, inline int, fallback int, runID uint64) uint64 {
	record := u.reviewRecord(task, startedAt, outcome, detail, response, inline, fallback, runID)
	id, err := u.deps.Reviews.Save(ctx, record)
	if err != nil {
		u.deps.Logger.Warn("리뷰 기록을 저장하지 못했습니다", "target", task.Target.Reference(), "error", err)
	}
	return id
}

func (u *UseCase) saveWithFindings(ctx context.Context, task job.ReviewJob, startedAt time.Time, outcome review.Outcome, detail string, response llm.Response, inline int, fallback int, runID uint64, lifecycleExpiresAt time.Time, findings []review.Finding) (uint64, error) {
	record := u.reviewRecord(task, startedAt, outcome, detail, response, inline, fallback, runID)
	id, err := u.deps.Reviews.SaveWithFindings(ctx, record, task.Target, findings, lifecycleExpiresAt)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (u *UseCase) reviewRecord(task job.ReviewJob, startedAt time.Time, outcome review.Outcome, detail string, response llm.Response, inline int, fallback int, runID uint64) review.Record {
	return review.Record{
		RunID:         runID,
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
}

func skipReason(task job.ReviewJob, config setting.RepoConfig, continuesRegisteredReview bool) (string, bool) {
	if !task.Trigger.IsAutomatic() {
		return "", false
	}
	if !config.Sandrone.AutoReview {
		return "sandrone.autoReview가 켜져 있지 않습니다", true
	}
	if task.Trigger == review.TriggerPullRequestDraftOpened && !config.Sandrone.AutoReviewOnDraft {
		return "sandrone.autoReviewOnDraft가 꺼져 있습니다", true
	}
	if task.Trigger == review.TriggerPullRequestPushed && continuesRegisteredReview {
		return "", false
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

func trackReviewModel(models map[string]struct{}, provider string, model string) {
	provider = strings.TrimSpace(provider)
	model = strings.TrimSpace(model)
	if provider == "" && model == "" {
		return
	}
	models[provider+"\x00"+model] = struct{}{}
}

func inventoryOf(files []pullrequest.ChangedFile) []pullrequest.ChangedFile {
	inventory := make([]pullrequest.ChangedFile, 0, len(files))
	for _, file := range files {
		inventory = append(inventory, pullrequest.ChangedFile{Path: file.Path, Additions: file.Additions, Deletions: file.Deletions})
	}
	return inventory
}
