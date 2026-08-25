package reviewpullrequest

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/it-play/sandrone-code-review-bot/internal/core/batching"
	"github.com/it-play/sandrone-code-review-bot/internal/core/job"
	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
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
	publicationAttempted := false
	startedAt := u.deps.Clock.Now()
	failureResponse := llm.Response{}
	fail := func(message string, cause error) error {
		if publicationAttempted {
			u.deps.Logger.Error(message, "target", task.Target.Reference(), "error", cause)
			return fmt.Errorf("%s: %w", message, cause)
		}
		recordFailure := !workflowRunCreated || activeRunID != 0
		return u.fail(ctx, task, startedAt, message, cause, activeRunID, recordFailure, failureResponse)
	}
	defer func() {
		if activeRunID == 0 || activeRunLease == "" {
			return
		}
		finishContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), reactionTimeout)
		defer cancel()
		if executeErr != nil && task.FinalAttempt && !publicationAttempted {
			if _, err := u.finishWorkflowWithOutcome(finishContext, activeRunID, activeRunLease, reviewworkflow.RunStatusFailed, review.OutcomeFailed, u.deps.Masker.Mask(executeErr.Error()), false); err != nil {
				u.deps.Logger.Error("실패한 리뷰 실행을 종료하지 못했습니다", "run", activeRunID, "error", err)
			}
		}
		if err := u.deps.Runs.ReleaseRun(finishContext, activeRunID, activeRunLease, u.deps.Clock.Now()); err != nil {
			u.deps.Logger.Warn("리뷰 실행 lease를 해제하지 못했습니다", "run", activeRunID, "error", err)
		}
	}()
	task.Instruction = u.deps.Masker.Mask(task.Instruction)
	target := task.Target
	request, err := u.deps.Source.PullRequest(ctx, target)
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
	stale := target.HeadSHA != "" && target.HeadSHA != request.HeadSHA
	if target.HeadSHA == "" {
		target.HeadSHA = request.HeadSHA
	}
	if target.BaseSHA == "" || !stale {
		target.BaseSHA = request.BaseSHA
	}
	target.BaseRef = request.BaseRef
	task.Target = target

	config, err := u.deps.Settings.RepoConfig(ctx, target)
	if err != nil {
		return fail("설정을 읽지 못했습니다", err)
	}
	classification := llm.DataClassificationPublic
	if request.Private {
		classification = llm.DataClassificationPrivateCode
	}
	maxCalls := u.deps.LLMMaxCalls
	if maxCalls < 1 {
		maxCalls = 12
	}
	verifierCallReserve := 0
	if maxCalls > 1 {
		verifierCallReserve = 1
	}
	reviewerCallLimit := maxCalls - verifierCallReserve
	callBudget := llm.NewExternalCallBudget(maxCalls)
	reviewRequest := llm.Request{
		Temperature:           config.Temperature,
		MaxOutputTokens:       config.MaxOutputTokens,
		Providers:             config.Sandrone.Providers,
		TaskRole:              llm.TaskRoleReviewer,
		DataClassification:    classification,
		ExternalCallBudget:    callBudget,
		ForceJSON:             true,
		RequireCompletePrompt: true,
		ResponseValidation:    llm.ResponseValidation{Policy: llm.ResponseValidationReviewResult},
	}
	policy := u.deps.Completer.PolicyHashInputs(reviewRequest)
	modelPolicyHash, err := hashJSON(policy)
	if err != nil {
		return fail("모델 정책 hash를 만들지 못했습니다", err)
	}
	runCandidate, err := newWorkflowRun(task, target, config, modelPolicyHash, startedAt, snapshotObservedAt)
	if err != nil {
		return fail("리뷰 실행 식별자를 만들지 못했습니다", err)
	}
	run, err := u.deps.Runs.CreateOrGetRun(ctx, runCandidate)
	if err != nil {
		return fail("리뷰 실행을 저장하지 못했습니다", err)
	}
	workflowRunCreated = true
	if run.Status.IsTerminal() {
		u.deps.Logger.Info("이미 종료된 리뷰 실행을 건너뜁니다", "target", target.Reference(), "run", run.ID, "status", run.Status)
		*outcome = reviewOutcomeOf(run.Status)
		return nil
	}
	resumePublishing := run.Status == reviewworkflow.RunStatusPublishing
	publicationAttempted = resumePublishing
	if stale {
		detail := "더 최신인 " + request.HeadSHA + " head가 있어 실행을 중단했습니다"
		var actualStatus reviewworkflow.RunStatus
		var finishErr error
		if resumePublishing {
			actualStatus, finishErr = u.supersedePublishedReview(ctx, target, reviewworkflow.PublicationMarker(run.Key), run.ID, "", request.HeadSHA, detail)
		} else {
			actualStatus, finishErr = u.finishWorkflowSupersededByHead(ctx, run.ID, "", request.HeadSHA, detail)
		}
		if finishErr != nil {
			return fail("오래된 리뷰 실행을 종료하지 못했습니다", finishErr)
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
		if _, finishErr := u.finishWorkflow(ctx, run.ID, "", reviewworkflow.RunStatusSkipped, reason, false); finishErr != nil {
			return fail("건너뛴 리뷰 실행을 저장하지 못했습니다", finishErr)
		}
		u.save(ctx, task, startedAt, review.OutcomeSkipped, reason, llm.Response{}, 0, 0, run.ID)
		*outcome = review.OutcomeSkipped
		return nil
	}
	registrationState, registrationErr := u.deps.Source.PullRequest(ctx, target)
	if registrationErr != nil {
		return fail("리뷰 실행 등록 전 Pull Request 상태를 확인하지 못했습니다", registrationErr)
	}
	if registrationState.HeadSHA != target.HeadSHA || registrationState.BaseSHA != target.BaseSHA {
		detail := "리뷰 실행 등록 전에 base 또는 head가 변경되었습니다"
		supersedingHeadSHA := changedHeadSHA(target.HeadSHA, registrationState.HeadSHA)
		var actualStatus reviewworkflow.RunStatus
		var finishErr error
		if resumePublishing {
			actualStatus, finishErr = u.supersedePublishedReview(ctx, target, reviewworkflow.PublicationMarker(run.Key), run.ID, "", supersedingHeadSHA, detail)
		} else {
			actualStatus, finishErr = u.finishWorkflowSupersededByHead(ctx, run.ID, "", supersedingHeadSHA, detail)
		}
		if finishErr != nil {
			return fail("오래된 리뷰 실행을 종료하지 못했습니다", finishErr)
		}
		u.save(ctx, task, startedAt, review.OutcomeSuperseded, detail, llm.Response{}, 0, 0, run.ID)
		*outcome = reviewOutcomeOf(actualStatus)
		return nil
	}
	runLease, leaseErr := u.deps.Runs.AcquireRun(ctx, run.ID, u.deps.Clock.Now(), u.deps.Clock.Now().Add(reviewRunLease))
	if errors.Is(leaseErr, reviewworkflow.ErrRunSuperseded) {
		detail := "더 최신인 리뷰 실행이 있어 중단했습니다"
		var actualStatus reviewworkflow.RunStatus
		var finishErr error
		if resumePublishing {
			actualStatus, finishErr = u.supersedePublishedReview(ctx, target, reviewworkflow.PublicationMarker(run.Key), run.ID, "", "", detail)
		} else {
			actualStatus, finishErr = u.finishWorkflow(ctx, run.ID, "", reviewworkflow.RunStatusSuperseded, detail, false)
		}
		if finishErr != nil {
			return fail("대체된 리뷰 실행을 종료하지 못했습니다", finishErr)
		}
		u.save(ctx, task, startedAt, review.OutcomeSuperseded, detail, llm.Response{}, 0, 0, run.ID)
		*outcome = reviewOutcomeOf(actualStatus)
		return nil
	}
	if errors.Is(leaseErr, reviewworkflow.ErrRunLeased) || errors.Is(leaseErr, reviewworkflow.ErrPublicationLeased) {
		u.deps.Logger.Info("다른 worker가 같은 리뷰 실행을 처리 중입니다", "target", target.Reference(), "run", run.ID)
		return leaseErr
	}
	if leaseErr != nil {
		return fail("리뷰 실행 lease를 얻지 못했습니다", leaseErr)
	}
	if runLease == "" {
		return nil
	}
	activeRunID = run.ID
	activeRunLease = runLease
	externalCalls := run.ExternalCalls
	if externalCalls < 0 {
		externalCalls = 0
	}
	if externalCalls > maxCalls {
		externalCalls = maxCalls
	}
	reserveExternalCall := func(limit int) func() error {
		return func() error {
			reserved, reserveErr := u.deps.Execution.ReserveExternalCall(ctx, run.ID, runLease, limit)
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
		marker := reviewworkflow.PublicationMarker(run.Key)
		storedPublication, storedPublicationFound, storedPublicationErr := u.deps.Publications.ReviewPublication(ctx, run.ID, runLease)
		if storedPublicationErr != nil {
			return fail("저장된 리뷰 게시 payload를 읽지 못했습니다", storedPublicationErr)
		}
		if storedPublicationFound && storedPublication.Marker != marker {
			return fail("저장된 리뷰 게시 marker가 실행과 일치하지 않습니다", fmt.Errorf("review run %d", run.ID))
		}
		published, publishedErr := u.deps.Publisher.PublicationExists(ctx, target, marker)
		if publishedErr != nil {
			return fail("기존 리뷰 게시 결과를 확인하지 못했습니다", publishedErr)
		}
		if published || storedPublicationFound {
			_, historyFound, historyErr := u.deps.Reviews.ByRunID(ctx, run.ID)
			if historyErr != nil {
				return fail("기존 리뷰 이력을 확인하지 못했습니다", historyErr)
			}
			if !historyFound {
				return fail("게시 상태에 대응하는 내부 리뷰 이력을 찾지 못했습니다", fmt.Errorf("review run %d", run.ID))
			}
			claimedAt := u.deps.Clock.Now()
			if claimErr := u.deps.Publications.ClaimPublication(ctx, run.ID, runLease, claimedAt, claimedAt.Add(reviewPublicationLease)); claimErr != nil {
				return fail("기존 리뷰 게시 결과를 조정하지 못했습니다", claimErr)
			}
			if published {
				expiresAt := run.StartedAt.Add(u.deps.Retention)
				if storedPublicationFound {
					expiresAt = storedPublication.ExpiresAt
				}
				if completeErr := u.completeReviewPublication(ctx, run.ID, runLease, marker, reviewworkflow.ReviewPublicationChannelReconciled, 0, expiresAt); completeErr != nil {
					return fail("기존 리뷰 게시 receipt를 완료하지 못했습니다", completeErr)
				}
			} else if storedPublication.Status == reviewworkflow.ReviewPublicationStatusPrepared {
				if publishErr := u.publishPreparedReview(ctx, target, run.ID, runLease, storedPublication); errors.Is(publishErr, publication.ErrTargetChanged) {
					detail := "저장된 리뷰 게시 payload 재개 직전에 base 또는 head가 변경되었습니다"
					supersedingHeadSHA := u.resolveSupersedingHeadSHA(ctx, target)
					actualStatus, finishErr := u.finishWorkflowSupersededByHead(ctx, run.ID, runLease, supersedingHeadSHA, detail)
					if finishErr != nil {
						return fail("재개 중 대체된 리뷰 실행을 종료하지 못했습니다", finishErr)
					}
					*outcome = reviewOutcomeOf(actualStatus)
					return nil
				} else if publishErr != nil {
					return fail("저장된 리뷰 게시 payload를 재게시하지 못했습니다", publishErr)
				}
			}
			finalization := reviewworkflow.ReviewPublicationFinalization{
				Status:           reviewworkflow.RunStatusPartial,
				Detail:           "legacy marker와 게시 receipt만 확인되어 보수적으로 부분 완료했습니다",
				AdvanceWatermark: false,
			}
			if storedPublicationFound && storedPublication.PayloadHash != "" {
				finalization = storedPublication.Finalization
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
		publicationAttempted = false
	}
	u.acknowledge(ctx, target, task)

	files, incremental, expectedFiles, err := u.collectFiles(ctx, target, request, task)
	if err != nil {
		return fail("변경 파일을 읽지 못했습니다", err)
	}
	if err := u.revalidateFindingOccurrences(ctx, target, files); err != nil {
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
		actualStatus, finishErr := u.finishWorkflow(ctx, run.ID, runLease, status, detail, advance)
		if finishErr != nil {
			return fail("리뷰 실행을 종료하지 못했습니다", finishErr)
		}
		if actualStatus == reviewworkflow.RunStatusSuperseded {
			u.save(ctx, task, startedAt, review.OutcomeSuperseded, detail, llm.Response{}, 0, 0, run.ID)
			*outcome = review.OutcomeSuperseded
			return nil
		}
		if actualStatus == reviewworkflow.RunStatusSkipped {
			u.notify(ctx, target, task, review.Notice{Kind: review.NoticeSkipped, Message: "리뷰할 변경 사항이 없습니다."})
			u.save(ctx, task, startedAt, review.OutcomeSkipped, detail, llm.Response{}, 0, 0, run.ID)
			*outcome = review.OutcomeSkipped
			return nil
		}
		u.notify(ctx, target, task, review.Notice{Kind: review.NoticeUnavailable, Message: "변경 diff를 확보하지 못해 리뷰를 완료하지 못했습니다."})
		u.save(ctx, task, startedAt, review.OutcomeUnavailable, detail, llm.Response{}, 0, 0, run.ID)
		*outcome = reviewOutcomeOf(actualStatus)
		return nil
	}
	loaded := u.loadSources(ctx, target, chosen.Files, config)
	pathMap := newPromptPathMap(files, u.deps.Masker.Mask)

	instructions, err := u.deps.Settings.Instructions(ctx, target, config)
	if err != nil {
		u.deps.Logger.Warn("지침 문서를 읽지 못했습니다", "target", target.Reference(), "error", err)
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
		ProviderLimit:     u.deps.Completer.PromptBudgetFor(reviewRequest),
		MaxReviewBatches:  config.Sandrone.MaxReviewBatches,
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
	if len(failed) > 0 && !task.FinalAttempt && !unitExecution.budgetExhausted {
		return fail("일부 리뷰 unit을 완료하지 못했습니다", fmt.Errorf("%d개 파일이 다음 시도에 남았습니다", len(failed)))
	}
	coverageSummary := reviewworkflow.SummarizeCoverage(coverage)
	runStatus := coverageSummary.TerminalStatus()
	if len(reviewed) == 0 || runStatus == reviewworkflow.RunStatusFailed {
		detail := coverageDetail(coverageSummary)
		projectionOutcome := reviewOutcomeOf(runStatus)
		if runStatus == reviewworkflow.RunStatusFailed && len(failed) > 0 {
			projectionOutcome = review.OutcomeFailed
		}
		actualStatus, finishErr := u.finishWorkflowWithOutcome(ctx, run.ID, runLease, runStatus, projectionOutcome, detail, false)
		if finishErr != nil {
			return fail("실패한 리뷰 실행을 종료하지 못했습니다", finishErr)
		}
		if actualStatus == reviewworkflow.RunStatusSuperseded {
			u.save(ctx, task, startedAt, review.OutcomeSuperseded, detail, response, 0, 0, run.ID)
			*outcome = review.OutcomeSuperseded
			return nil
		}
		if len(failed) == 0 {
			u.notify(ctx, target, task, review.Notice{Kind: review.NoticeUnavailable, Message: "검토 가능한 전체 diff를 확보하지 못해 리뷰를 완료하지 못했습니다."})
			u.save(ctx, task, startedAt, review.OutcomeUnavailable, detail, response, 0, 0, run.ID)
			*outcome = reviewOutcomeOf(actualStatus)
			return nil
		}
		if unitExecution.budgetExhausted {
			u.notify(ctx, target, task, review.Notice{Kind: review.NoticeUnavailable, Message: "정해진 호출 예산 안에서 유효한 리뷰 결과를 만들지 못했습니다."})
			u.save(ctx, task, startedAt, review.OutcomeFailed, detail, response, 0, 0, run.ID)
			*outcome = review.OutcomeFailed
			return nil
		}
		return fail("유효한 리뷰 결과를 만들지 못했습니다", fmt.Errorf("모든 리뷰 unit이 실패했습니다"))
	}

	reviewRequest.ExternalCallBudget = llm.NewScopedDurableExternalCallBudget(maxCalls, unitExecution.externalCalls, maxCalls, reserveExternalCall(maxCalls))
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
			if _, finishErr := u.finishWorkflow(ctx, run.ID, runLease, reviewworkflow.RunStatusSuperseded, detail, false); finishErr != nil {
				return fail("대체된 리뷰 실행을 종료하지 못했습니다", finishErr)
			}
			u.save(ctx, task, startedAt, review.OutcomeSuperseded, detail, response, 0, 0, run.ID)
			*outcome = review.OutcomeSuperseded
			return nil
		}
		return fail("게시 전 리뷰 실행 lease를 갱신하지 못했습니다", leaseErr)
	}
	latest, latestErr := u.deps.Source.PullRequest(ctx, target)
	if latestErr != nil {
		return fail("게시 전 Pull Request 상태를 확인하지 못했습니다", latestErr)
	}
	if latest.HeadSHA != target.HeadSHA || latest.BaseSHA != target.BaseSHA {
		detail := "게시 전에 base 또는 head가 변경되었습니다"
		if _, finishErr := u.finishWorkflowSupersededByHead(ctx, run.ID, runLease, changedHeadSHA(target.HeadSHA, latest.HeadSHA), detail); finishErr != nil {
			return fail("오래된 리뷰 실행을 종료하지 못했습니다", finishErr)
		}
		u.save(ctx, task, startedAt, review.OutcomeSuperseded, detail, response, 0, 0, run.ID)
		*outcome = review.OutcomeSuperseded
		return nil
	}

	publicationClaimedAt := u.deps.Clock.Now()
	if claimErr := u.deps.Publications.ClaimPublication(ctx, run.ID, runLease, publicationClaimedAt, publicationClaimedAt.Add(reviewPublicationLease)); claimErr != nil {
		if errors.Is(claimErr, reviewworkflow.ErrRunSuperseded) {
			detail := "게시 권한을 얻기 전에 더 최신인 리뷰 실행이 확인되었습니다"
			if _, finishErr := u.finishWorkflow(ctx, run.ID, runLease, reviewworkflow.RunStatusSuperseded, detail, false); finishErr != nil {
				return fail("대체된 리뷰 실행을 종료하지 못했습니다", finishErr)
			}
			u.save(ctx, task, startedAt, review.OutcomeSuperseded, detail, response, 0, 0, run.ID)
			*outcome = review.OutcomeSuperseded
			return nil
		}
		return fail("리뷰 게시 권한을 얻지 못했습니다", claimErr)
	}
	confirmed, confirmedErr := u.deps.Source.PullRequest(ctx, target)
	if confirmedErr != nil {
		return fail("게시 직전 Pull Request 상태를 확인하지 못했습니다", confirmedErr)
	}
	if confirmed.HeadSHA != target.HeadSHA || confirmed.BaseSHA != target.BaseSHA {
		detail := "게시 권한을 얻은 뒤 base 또는 head가 변경되었습니다"
		if _, finishErr := u.finishWorkflowSupersededByHead(ctx, run.ID, runLease, changedHeadSHA(target.HeadSHA, confirmed.HeadSHA), detail); finishErr != nil {
			return fail("오래된 리뷰 실행을 종료하지 못했습니다", finishErr)
		}
		u.save(ctx, task, startedAt, review.OutcomeSuperseded, detail, response, 0, 0, run.ID)
		*outcome = review.OutcomeSuperseded
		return nil
	}

	provisionalDetail := "GitHub 게시 결과 확인 대기 중"
	if _, persistErr := u.saveWithFindings(ctx, task, startedAt, review.OutcomeUnavailable, provisionalDetail, response, len(placed.Inline()), len(placed.Fallback()), run.ID, run.StartedAt.Add(u.deps.Retention), placed.Findings); persistErr != nil {
		return fail("게시 전 리뷰 이력과 지적을 저장하지 못했습니다", persistErr)
	}
	publicationAttempted = true
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
	if publishErr := u.prepareAndPublishReview(ctx, target, run.ID, runLease, publicationMarker, view, placed, attribution, style, finalization); errors.Is(publishErr, publication.ErrTargetChanged) {
		detail := "리뷰 게시 요청 직전에 base 또는 head가 변경되었습니다"
		supersedingHeadSHA := u.resolveSupersedingHeadSHA(ctx, target)
		actualStatus, finishErr := u.finishWorkflowSupersededByHead(ctx, run.ID, runLease, supersedingHeadSHA, detail)
		if finishErr != nil {
			return fail("게시 직전에 대체된 리뷰 실행을 종료하지 못했습니다", finishErr)
		}
		u.save(ctx, task, startedAt, review.OutcomeSuperseded, detail, response, 0, 0, run.ID)
		*outcome = reviewOutcomeOf(actualStatus)
		return nil
	} else if publishErr != nil {
		return fail("리뷰 게시 결과를 확인하지 못했습니다", publishErr)
	}
	actualStatus, finishErr := u.finalizePublishedReview(ctx, target, run.ID, runLease, publicationMarker, finalization.Status, finalization.Detail, finalization.AdvanceWatermark)
	if finishErr != nil {
		return fail("리뷰 실행을 완료하지 못했습니다", finishErr)
	}
	*outcome = reviewOutcomeOf(actualStatus)
	return nil
}

func (u *UseCase) acknowledge(ctx context.Context, target pullrequest.Target, task job.ReviewJob) {
	if !task.Trigger.IsAutomatic() {
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

func (u *UseCase) loadSources(ctx context.Context, target pullrequest.Target, files []pullrequest.ChangedFile, config setting.RepoConfig) []pullrequest.ChangedFile {
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
		if config.IncludeSources && limit > 0 {
			content, err := u.deps.Source.FileContent(ctx, target, file.Path, target.HeadSHA)
			if err == nil {
				content = u.deps.Masker.Mask(content)
				if len(content) > limit {
					content = truncateUTF8(content, limit)
					file.Truncated = true
				}
				file.Content = content
			}
		}
		loaded = append(loaded, file)
	}
	return loaded
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

func (u *UseCase) supersedePublishedReview(ctx context.Context, target pullrequest.Target, marker string, runID uint64, leaseToken string, supersedingHeadSHA string, detail string) (reviewworkflow.RunStatus, error) {
	if err := u.deps.Publisher.InvalidatePublication(ctx, target, marker, invalidatedReviewBody(detail)); err != nil {
		return reviewworkflow.RunStatusPublishing, err
	}
	return u.finishWorkflowSupersededByHead(ctx, runID, leaseToken, supersedingHeadSHA, detail)
}

func (u *UseCase) resolveSupersedingHeadSHA(ctx context.Context, target pullrequest.Target) string {
	current, err := u.deps.Source.PullRequest(ctx, target)
	if err != nil {
		u.deps.Logger.Warn("대체 head를 확인하지 못했습니다", "target", target.Reference(), "error", err)
		return ""
	}
	return changedHeadSHA(target.HeadSHA, current.HeadSHA)
}

func (u *UseCase) notify(ctx context.Context, target pullrequest.Target, task job.ReviewJob, notice review.Notice) {
	if task.Trigger.IsAutomatic() {
		return
	}
	if _, err := u.deps.Publisher.CreateComment(ctx, target, u.deps.Renderer.NoticeBody(notice)); err != nil {
		u.deps.Logger.Warn("안내 코멘트를 남기지 못했습니다", "target", target.Reference(), "error", err)
	}
}

func (u *UseCase) fail(ctx context.Context, task job.ReviewJob, startedAt time.Time, message string, cause error, runID uint64, recordFailure bool, response llm.Response) error {
	u.deps.Logger.Error(message, "target", task.Target.Reference(), "error", cause)
	if notice, announce := failureNotice(task.Attempt, task.FinalAttempt); announce {
		u.announce(ctx, task.Target, notice)
	}
	if task.FinalAttempt && recordFailure {
		u.save(ctx, task, startedAt, review.OutcomeFailed, message, response, 0, 0, runID)
	}
	return fmt.Errorf("%s: %w", message, cause)
}

func (u *UseCase) announce(ctx context.Context, target pullrequest.Target, notice review.Notice) {
	if _, err := u.deps.Publisher.CreateComment(ctx, target, u.deps.Renderer.NoticeBody(notice)); err != nil {
		u.deps.Logger.Warn("실패 안내를 남기지 못했습니다", "target", target.Reference(), "error", err)
	}
}

func failureNotice(attempt int, final bool) (review.Notice, bool) {
	switch {
	case final:
		return review.Notice{Kind: review.NoticeFailed, Message: "리뷰를 완료하지 못했습니다. 재시도했지만 해결되지 않아 중단합니다."}, true
	case attempt == 0:
		return review.Notice{Kind: review.NoticeRetrying, Message: "리뷰를 완료하지 못했습니다. 잠시 후 다시 시도합니다."}, true
	default:
		return review.Notice{}, false
	}
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
