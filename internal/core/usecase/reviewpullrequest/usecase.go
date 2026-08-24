package reviewpullrequest

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/it-play/sandrone-code-review-bot/internal/core/batching"
	"github.com/it-play/sandrone-code-review-bot/internal/core/dedupe"
	"github.com/it-play/sandrone-code-review-bot/internal/core/job"
	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
	"github.com/it-play/sandrone-code-review-bot/internal/core/mapping"
	"github.com/it-play/sandrone-code-review-bot/internal/core/publication"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewanalysis"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
	"github.com/it-play/sandrone-code-review-bot/internal/core/selection"
	"github.com/it-play/sandrone-code-review-bot/internal/core/setting"
	"github.com/it-play/sandrone-code-review-bot/internal/core/usecase/reviewpublication"
)

const reactionTimeout = 5 * time.Second
const reviewRunLease = 20 * time.Minute
const reviewPublicationLease = 30 * time.Minute

type UseCase struct {
	deps         Dependencies
	publications *reviewpublication.PreparedPublisher
	unitExecutor *reviewUnitExecutor
}

func New(deps Dependencies) *UseCase {
	return &UseCase{
		deps: deps,
		publications: reviewpublication.New(reviewpublication.Dependencies{
			Publisher: deps.Publisher,
			Receipts:  deps.Workflows,
			Clock:     deps.Clock,
			Logger:    deps.Logger,
		}),
		unitExecutor: newReviewUnitExecutor(reviewUnitExecutorDependencies{
			workflows: deps.Workflows,
			completer: deps.Completer,
			tools:     deps.Tools,
			masker:    deps.Masker,
			clock:     deps.Clock,
			parser:    deps.Parser,
			logger:    deps.Logger,
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
		if err := u.deps.Workflows.ReleaseRun(finishContext, activeRunID, activeRunLease, u.deps.Clock.Now()); err != nil {
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
	run, err := u.deps.Workflows.CreateOrGetRun(ctx, runCandidate)
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
			actualStatus, finishErr = u.supersedePublishedReview(ctx, target, reviewworkflow.PublicationMarker(run.Key), run.ID, "", detail)
		} else {
			actualStatus, finishErr = u.finishWorkflow(ctx, run.ID, "", reviewworkflow.RunStatusSuperseded, detail, false)
		}
		if finishErr != nil {
			return fail("오래된 리뷰 실행을 종료하지 못했습니다", finishErr)
		}
		u.save(ctx, task, startedAt, review.OutcomeSuperseded, detail, llm.Response{}, 0, 0, run.ID)
		*outcome = reviewOutcomeOf(actualStatus)
		return nil
	}
	if reason, skipped := skipReason(task, config); skipped {
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
		var actualStatus reviewworkflow.RunStatus
		var finishErr error
		if resumePublishing {
			actualStatus, finishErr = u.supersedePublishedReview(ctx, target, reviewworkflow.PublicationMarker(run.Key), run.ID, "", detail)
		} else {
			actualStatus, finishErr = u.finishWorkflow(ctx, run.ID, "", reviewworkflow.RunStatusSuperseded, detail, false)
		}
		if finishErr != nil {
			return fail("오래된 리뷰 실행을 종료하지 못했습니다", finishErr)
		}
		u.save(ctx, task, startedAt, review.OutcomeSuperseded, detail, llm.Response{}, 0, 0, run.ID)
		*outcome = reviewOutcomeOf(actualStatus)
		return nil
	}
	runLease, leaseErr := u.deps.Workflows.AcquireRun(ctx, run.ID, u.deps.Clock.Now(), u.deps.Clock.Now().Add(reviewRunLease))
	if errors.Is(leaseErr, reviewworkflow.ErrRunSuperseded) {
		detail := "더 최신인 리뷰 실행이 있어 중단했습니다"
		var actualStatus reviewworkflow.RunStatus
		var finishErr error
		if resumePublishing {
			actualStatus, finishErr = u.supersedePublishedReview(ctx, target, reviewworkflow.PublicationMarker(run.Key), run.ID, "", detail)
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
	callBudget = llm.NewDurableExternalCallBudget(maxCalls, func() bool {
		reserved, reserveErr := u.deps.Workflows.ReserveExternalCall(ctx, run.ID, runLease, maxCalls)
		if reserveErr != nil {
			u.deps.Logger.Warn("리뷰 실행 외부 호출 예산을 예약하지 못했습니다", "run", run.ID, "error", reserveErr)
			return false
		}
		return reserved
	})
	reviewRequest.ExternalCallBudget = callBudget
	if resumePublishing {
		marker := reviewworkflow.PublicationMarker(run.Key)
		storedPublication, storedPublicationFound, storedPublicationErr := u.deps.Workflows.ReviewPublication(ctx, run.ID, runLease)
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
			if claimErr := u.deps.Workflows.ClaimPublication(ctx, run.ID, runLease, claimedAt, claimedAt.Add(reviewPublicationLease)); claimErr != nil {
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
					actualStatus, finishErr := u.finishWorkflow(ctx, run.ID, runLease, reviewworkflow.RunStatusSuperseded, detail, false)
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
		if resumeErr := u.deps.Workflows.ResumeRun(ctx, run.ID, runLease, u.deps.Clock.Now()); resumeErr != nil {
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
		if _, planErr := u.deps.Workflows.SavePlan(ctx, run.ID, runLease, nil, coverage, u.deps.Clock.Now()); planErr != nil {
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
		target:           target,
		runID:            run.ID,
		runConfigHash:    run.ConfigHash,
		runLease:         runLease,
		config:           config,
		planner:          planner,
		plan:             plan,
		promptConfig:     promptConfig,
		units:            units,
		coverage:         coverage,
		request:          reviewRequest,
		paths:            pathMap,
		includeFileNotes: includeFileNotes,
	})
	failureResponse = unitExecution.response
	if unitExecutionErr != nil {
		return fail(unitExecution.errorMessage, unitExecutionErr)
	}
	gathered := unitExecution.review
	reduced := unitExecution.reduced
	response := unitExecution.response
	usage := response.Usage
	toolExecutions := response.ToolExecutions
	modelsUsed := unitExecution.modelsUsed
	multipleModels := unitExecution.multipleModels
	reviewed := unitExecution.reviewed
	failed := unitExecution.failed
	schemaDropped := unitExecution.schemaDropped
	evidenceReport := unitExecution.evidenceReport
	reusedFindings := unitExecution.reusedFindings
	reviewerProviders := unitExecution.reviewerProviders
	captureFailureResponse := func() {
		response.Usage = usage
		response.ToolExecutions = toolExecutions
		if multipleModels || len(modelsUsed) > 1 {
			response.Provider = "multiple"
			response.Model = "multiple"
			response.ModelLabel = "복수 모델"
		}
		failureResponse = response
	}
	if len(failed) > 0 && !task.FinalAttempt {
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
		return fail("유효한 리뷰 결과를 만들지 못했습니다", fmt.Errorf("모든 리뷰 unit이 실패했습니다"))
	}

	findings := selection.SeverityFilter{Minimum: config.MinSeverity}.Apply(gathered.Findings)
	afterSeverity := len(findings)
	var finalEvidenceReport reviewanalysis.EvidenceReport
	findings, finalEvidenceReport = (reviewanalysis.EvidenceVerifier{}).VerifyWithReport(findings, reviewed)
	known, err := u.deps.Findings.Fingerprints(ctx, target)
	if err != nil {
		return fail("기존 지적을 읽지 못했습니다", err)
	}
	findings, duplicates := dedupe.DuplicateFilter{Known: known}.Apply(findings)
	findings, omittedFindings := publishedFindingBudget(findings)
	findings, verifierResponse, verifierRejected, verifierUsed, verificationUnavailable, verificationErr := u.verifyFindings(ctx, run.ID, runLease, findings, reviewed, reviewRequest, reviewerProviders, pathMap)
	usage = usage.Add(verifierResponse.Usage)
	toolExecutions += verifierResponse.ToolExecutions
	trackReviewModel(modelsUsed, verifierResponse.Provider, verifierResponse.Model)
	multipleModels = multipleModels || verifierResponse.ModelLabel == "복수 모델"
	if verifierResponse.Provider != "" || verifierResponse.Model != "" {
		response.Provider = verifierResponse.Provider
		response.Model = verifierResponse.Model
		response.ModelLabel = verifierResponse.ModelLabel
	}
	captureFailureResponse()
	if verificationErr != nil {
		return fail("독립 finding verifier 결과를 저장하지 못했습니다", verificationErr)
	}
	if verificationUnavailable {
		runStatus = reviewworkflow.RunStatusPartial
	}
	var finalReduced int
	findings, finalReduced = (dedupe.RootCauseReducer{}).Apply(findings)
	findings = mapping.PositionMapper{MaxInline: config.MaxInlineComments}.Map(findings, reviewed)
	u.deps.Logger.Info("지적 집계",
		"target", target.Reference(),
		"collected", len(gathered.Findings),
		"after_severity", afterSeverity,
		"schema_dropped", schemaDropped,
		"checkpoint_reused_findings", reusedFindings,
		"evidence_exact", evidenceReport.Exact,
		"evidence_normalized", evidenceReport.Normalized,
		"evidence_reanchored", evidenceReport.Reanchored,
		"evidence_unknown_file", evidenceReport.UnknownFile,
		"evidence_invalid_span", evidenceReport.InvalidSpan,
		"evidence_non_added_span", evidenceReport.NonAddedSpan,
		"evidence_not_found", evidenceReport.NotFound,
		"evidence_ambiguous", evidenceReport.Ambiguous,
		"evidence_accepted", evidenceReport.Accepted(),
		"evidence_rejected", evidenceReport.Dropped(),
		"final_evidence_exact", finalEvidenceReport.Exact,
		"final_evidence_normalized", finalEvidenceReport.Normalized,
		"final_evidence_reanchored", finalEvidenceReport.Reanchored,
		"final_evidence_rejected", finalEvidenceReport.Dropped(),
		"root_causes_collapsed", reduced+finalReduced,
		"verifier_used", verifierUsed,
		"verifier_rejected", verifierRejected,
		"verification_unavailable", verificationUnavailable,
		"omitted_by_budget", omittedFindings,
		"duplicates", duplicates,
		"final", len(findings),
		"min_severity", string(config.MinSeverity))

	placed := review.Result{Summary: gathered.Summary, Findings: findings}
	response.Usage = usage
	response.ToolExecutions = toolExecutions
	if multipleModels || len(modelsUsed) > 1 {
		response.Provider = "multiple"
		response.Model = "multiple"
		response.ModelLabel = "복수 모델"
	}
	attribution := attributionOf(response)
	style := review.Style{Emoji: config.Emoji, Tone: string(config.Tone)}
	view := review.SummaryView{
		Summary:                 placed.Summary,
		Fallback:                placed.Fallback(),
		InlineCount:             len(placed.Inline()),
		Attribution:             attribution,
		Style:                   style,
		Trigger:                 task.Trigger,
		Incremental:             incremental,
		SkippedDup:              duplicates,
		OmittedFindings:         omittedFindings,
		VerificationUnavailable: verificationUnavailable,
		Unreviewed:              unreviewedOf(chosen.Skipped, plan.Overflow, plan.Oversized, failed, coverage),
		Coverage: review.CoverageView{
			Total:    coverageSummary.Total,
			Reviewed: coverageSummary.Reviewed,
			Failed:   coverageSummary.Failed,
			Deferred: coverageSummary.Deferred,
			Skipped:  coverageSummary.Skipped,
			Pending:  coverageSummary.Pending,
			Status:   string(runStatus),
		},
	}
	heartbeatAt := u.deps.Clock.Now()
	if leaseErr := u.deps.Workflows.RenewRun(ctx, run.ID, runLease, heartbeatAt, heartbeatAt.Add(reviewRunLease)); leaseErr != nil {
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
		if _, finishErr := u.finishWorkflow(ctx, run.ID, runLease, reviewworkflow.RunStatusSuperseded, detail, false); finishErr != nil {
			return fail("오래된 리뷰 실행을 종료하지 못했습니다", finishErr)
		}
		u.save(ctx, task, startedAt, review.OutcomeSuperseded, detail, response, 0, 0, run.ID)
		*outcome = review.OutcomeSuperseded
		return nil
	}

	publicationClaimedAt := u.deps.Clock.Now()
	if claimErr := u.deps.Workflows.ClaimPublication(ctx, run.ID, runLease, publicationClaimedAt, publicationClaimedAt.Add(reviewPublicationLease)); claimErr != nil {
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
		if _, finishErr := u.finishWorkflow(ctx, run.ID, runLease, reviewworkflow.RunStatusSuperseded, detail, false); finishErr != nil {
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
		actualStatus, finishErr := u.finishWorkflow(ctx, run.ID, runLease, reviewworkflow.RunStatusSuperseded, detail, false)
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

func (u *UseCase) supersedePublishedReview(ctx context.Context, target pullrequest.Target, marker string, runID uint64, leaseToken string, detail string) (reviewworkflow.RunStatus, error) {
	if err := u.deps.Publisher.InvalidatePublication(ctx, target, marker, invalidatedReviewBody(detail)); err != nil {
		return reviewworkflow.RunStatusPublishing, err
	}
	return u.finishWorkflow(ctx, runID, leaseToken, reviewworkflow.RunStatusSuperseded, detail, false)
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
	if notice, announce := failureNotice(task.Attempt, task.FinalAttempt, message); announce {
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

func unreviewedOf(overCount []pullrequest.ChangedFile, overflow []pullrequest.ChangedFile, oversized []pullrequest.ChangedFile, failed []pullrequest.ChangedFile, coverage []reviewworkflow.CoverageItem) []review.UnreviewedFile {
	groups := []struct {
		files  []pullrequest.ChangedFile
		reason string
	}{
		{overCount, "리뷰 대상 파일 수 상한 초과"},
		{overflow, "리뷰 배치 수 상한 초과"},
		{oversized, "모델 입력 한도 초과"},
		{failed, "모델 호출 실패"},
	}
	unreviewed := make([]review.UnreviewedFile, 0)
	seen := map[string]struct{}{}
	for _, group := range groups {
		for _, file := range group.files {
			if _, found := seen[file.Path]; found {
				continue
			}
			seen[file.Path] = struct{}{}
			unreviewed = append(unreviewed, review.UnreviewedFile{
				Path:      file.Path,
				Additions: file.Additions,
				Deletions: file.Deletions,
				Reason:    group.reason,
			})
		}
	}
	coverageReasons := map[string]string{
		"patch_unavailable":        "GitHub diff를 가져오지 못함",
		"patch_truncated":          "파일 diff 분량 상한 초과",
		"incomplete_hunk":          "완전한 diff hunk를 가져오지 못함",
		"patch_incomplete":         "파일 diff 일부 누락",
		"file_manifest_incomplete": "PR 변경 파일 목록 일부 누락",
		"prompt_limit":             "모델 입력 한도 초과",
		"unplanned":                "리뷰 unit에 배정되지 않음",
	}
	for _, item := range coverage {
		if item.Status != reviewworkflow.CoverageStatusDeferred && item.Status != reviewworkflow.CoverageStatusFailed {
			continue
		}
		reason, found := coverageReasons[item.Reason]
		if !found {
			continue
		}
		path := item.Path
		if path == "" {
			path = "PR diff manifest"
		}
		if _, found := seen[path]; found {
			continue
		}
		seen[path] = struct{}{}
		unreviewed = append(unreviewed, review.UnreviewedFile{Path: path, Reason: reason})
	}
	sort.SliceStable(unreviewed, func(left, right int) bool {
		return unreviewed[left].Path < unreviewed[right].Path
	})
	return unreviewed
}
