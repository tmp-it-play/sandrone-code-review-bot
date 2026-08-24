package reviewpullrequest

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
	"github.com/it-play/sandrone-code-review-bot/internal/core/parsing"
	"github.com/it-play/sandrone-code-review-bot/internal/core/port/outbound"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewanalysis"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
	"github.com/it-play/sandrone-code-review-bot/internal/core/selection"
)

const reviewUnitLease = 20 * time.Minute

type reviewUnitExecutor struct {
	deps reviewUnitExecutorDependencies
}

func newReviewUnitExecutor(deps reviewUnitExecutorDependencies) *reviewUnitExecutor {
	return &reviewUnitExecutor{deps: deps}
}

func (e *reviewUnitExecutor) execute(ctx context.Context, input reviewUnitExecution) (reviewUnitExecutionResult, error) {
	result := reviewUnitExecutionResult{
		modelsUsed:        map[string]struct{}{},
		reviewed:          make([]pullrequest.ChangedFile, 0),
		failed:            make([]pullrequest.ChangedFile, 0),
		reviewerProviders: map[string]struct{}{},
	}
	usage := llm.Usage{}
	response := llm.Response{}
	toolExecutions := 0
	batchResults := make([]review.Result, 0, len(input.plan.Batches))
	captureResponse := func() {
		response.Usage = usage
		response.ToolExecutions = toolExecutions
		if result.multipleModels || len(result.modelsUsed) > 1 {
			response.Provider = "multiple"
			response.Model = "multiple"
			response.ModelLabel = "복수 모델"
		}
		result.response = response
	}
	fail := func(message string, cause error) (reviewUnitExecutionResult, error) {
		captureResponse()
		result.errorMessage = message
		return result, cause
	}
	if _, err := e.deps.workflows.SavePlan(ctx, input.runID, input.runLease, input.units, input.coverage, e.deps.clock.Now()); err != nil {
		return fail("리뷰 실행 계획을 저장하지 못했습니다", err)
	}
	for index, batch := range input.plan.Batches {
		unit := input.units[index]
		messages := llm.MaskMessages(input.planner.Messages(batch, input.promptConfig), e.deps.masker.Mask)
		batchRequest := input.request.WithMessages(messages)
		promptFiles := input.planner.maskedFiles(batch)
		if input.includeFileNotes {
			batchRequest.ResponseValidation.RequiredPaths = make([]string, 0, len(promptFiles))
			for _, file := range promptFiles {
				batchRequest.ResponseValidation.RequiredPaths = append(batchRequest.ResponseValidation.RequiredPaths, file.Path)
			}
			sort.Strings(batchRequest.ResponseValidation.RequiredPaths)
		}
		batchRequest.ResponseValidation.Validator = reviewGroundingValidator(promptFiles, input.config.MinSeverity, e.deps.masker.Mask)
		executor := e.toolExecutor(input.target, input.config.MaxExtraReads, input.paths)
		var toolDefinitions []llm.Tool
		if executor != nil {
			toolDefinitions = executor.Definitions()
		}
		inputHash, err := reviewUnitInputHash(input.target.FullName(), input.runConfigHash, unit.Hash, batchRequest, e.deps.completer.PolicyHashInputs(batchRequest), input.config.MaxExtraReads, toolDefinitions)
		if err != nil {
			return fail("리뷰 unit 입력 hash를 만들지 못했습니다", err)
		}
		unitStartedAt := e.deps.clock.Now()
		claim, err := e.deps.workflows.StartUnit(ctx, input.runID, input.runLease, unit.Hash, inputHash, unitStartedAt, unitStartedAt.Add(reviewUnitLease))
		if err != nil {
			return fail("리뷰 unit을 시작하지 못했습니다", err)
		}
		if claim.Completed {
			finishedAt := claim.Result.FinishedAt
			if finishedAt.IsZero() {
				finishedAt = unitStartedAt
			}
			batchResults = append(batchResults, claim.Result.Review)
			result.reusedFindings += len(claim.Result.Review.Findings)
			if !claim.Reused {
				usage = usage.Add(claim.Result.Usage)
				toolExecutions += claim.Result.ToolExecutions
			}
			response.Provider = claim.Result.Provider
			response.Model = claim.Result.Model
			response.ModelLabel = ""
			if claim.Result.MultipleModels {
				response.ModelLabel = "복수 모델"
			}
			trackReviewModel(result.modelsUsed, claim.Result.Provider, claim.Result.Model)
			result.multipleModels = result.multipleModels || claim.Result.MultipleModels
			if claim.Result.Provider != "" {
				result.reviewerProviders[claim.Result.Provider] = struct{}{}
			}
			captureResponse()
			transitionCoverage(input.coverage, unit.Hash, reviewworkflow.CoverageStatusReviewed, &finishedAt)
			result.reviewed = append(result.reviewed, batch...)
			e.deps.logger.Info("저장된 리뷰 unit 결과를 재사용했습니다", "target", input.target.Reference(), "batch", index+1, "cross_run", claim.Reused)
			continue
		}
		usage = usage.Add(claim.Result.Usage)
		toolExecutions += claim.Result.ToolExecutions
		trackReviewModel(result.modelsUsed, claim.Result.Provider, claim.Result.Model)
		if claim.Result.MultipleModels || claim.Result.Provider == "multiple" || claim.Result.Model == "multiple" {
			result.multipleModels = true
		}
		if claim.Result.Provider != "" || claim.Result.Model != "" {
			response.Provider = claim.Result.Provider
			response.Model = claim.Result.Model
			if claim.Result.MultipleModels {
				response.ModelLabel = "복수 모델"
			}
		}
		captureResponse()

		batchResponse, batchErr := e.deps.completer.Complete(ctx, batchRequest, executor)
		if batchErr != nil {
			maskedBatchErr := errors.New(e.deps.masker.Mask(batchErr.Error()))
			usage = usage.Add(batchResponse.Usage)
			toolExecutions += batchResponse.ToolExecutions
			response.Provider = batchResponse.Provider
			response.Model = batchResponse.Model
			response.ModelLabel = batchResponse.ModelLabel
			trackReviewModel(result.modelsUsed, batchResponse.Provider, batchResponse.Model)
			result.multipleModels = result.multipleModels || batchResponse.ModelLabel == "복수 모델"
			captureResponse()
			e.deps.logger.Warn("일부 배치를 리뷰하지 못했습니다", "target", input.target.Reference(), "batch", index+1, "error", maskedBatchErr)
			unitFinishedAt := e.deps.clock.Now()
			if err := e.finishUnitCheckpoint(ctx, input.runID, input.runLease, unit.Hash, claim.LeaseToken, reviewworkflow.UnitResult{
				Status:         reviewworkflow.UnitStatusFailed,
				InputHash:      inputHash,
				Provider:       batchResponse.Provider,
				Model:          batchResponse.Model,
				MultipleModels: batchResponse.ModelLabel == "복수 모델",
				Usage:          batchResponse.Usage,
				ToolExecutions: batchResponse.ToolExecutions,
				Error:          maskedBatchErr.Error(),
				FinishedAt:     unitFinishedAt,
			}); err != nil {
				return fail("실패한 리뷰 unit을 저장하지 못했습니다", err)
			}
			transitionCoverage(input.coverage, unit.Hash, reviewworkflow.CoverageStatusFailed, nil)
			result.failed = append(result.failed, batch...)
			continue
		}
		usage = usage.Add(batchResponse.Usage)
		toolExecutions += batchResponse.ToolExecutions
		response = batchResponse
		trackReviewModel(result.modelsUsed, batchResponse.Provider, batchResponse.Model)
		result.multipleModels = result.multipleModels || batchResponse.ModelLabel == "복수 모델"
		captureResponse()
		if batchResponse.Provider != "" {
			result.reviewerProviders[batchResponse.Provider] = struct{}{}
		}

		contentLength := len(batchResponse.Content)
		batchResult := review.Result{}
		report := parsing.Report{}
		var parseErr error
		if !batchResponse.Completed() {
			parseErr = fmt.Errorf("모델 응답이 완료되지 않았습니다: %s", batchResponse.FinishReason)
		} else {
			batchResult, report, parseErr = e.deps.parser.Parse(batchResponse.Content)
			batchResult = input.paths.RestoreResult(batchResult)
			batchResult = maskReviewResult(batchResult, e.deps.masker.Mask)
		}
		batchResponse.Content = ""
		response.Content = ""
		result.response.Content = ""
		if parseErr == nil {
			batchResult = boundedBatchResult(batch, batchResult, input.includeFileNotes)
			parseErr = validateBatchResult(batch, batchResult, report, input.includeFileNotes)
		}
		if parseErr != nil {
			e.deps.logger.Warn("일부 배치의 응답을 해석하지 못했습니다", "target", input.target.Reference(), "batch", index+1, "error", parseErr)
			unitFinishedAt := e.deps.clock.Now()
			if err := e.finishUnitCheckpoint(ctx, input.runID, input.runLease, unit.Hash, claim.LeaseToken, reviewworkflow.UnitResult{
				Status:         reviewworkflow.UnitStatusFailed,
				InputHash:      inputHash,
				Provider:       batchResponse.Provider,
				Model:          batchResponse.Model,
				MultipleModels: batchResponse.ModelLabel == "복수 모델",
				Usage:          batchResponse.Usage,
				ToolExecutions: batchResponse.ToolExecutions,
				Error:          e.deps.masker.Mask(parseErr.Error()),
				FinishedAt:     unitFinishedAt,
			}); err != nil {
				return fail("해석 실패한 리뷰 unit을 저장하지 못했습니다", err)
			}
			transitionCoverage(input.coverage, unit.Hash, reviewworkflow.CoverageStatusFailed, nil)
			result.failed = append(result.failed, batch...)
			continue
		}
		result.schemaDropped += report.Dropped
		minimumSeverityCandidates := len((selection.SeverityFilter{Minimum: input.config.MinSeverity}).Apply(batchResult.Findings))
		var batchEvidenceReport reviewanalysis.EvidenceReport
		batchResult.Findings, batchEvidenceReport = (reviewanalysis.EvidenceVerifier{}).VerifyWithReport(batchResult.Findings, batch)
		result.evidenceReport = result.evidenceReport.Add(batchEvidenceReport)
		batchResult.Findings = (selection.SeverityFilter{Minimum: input.config.MinSeverity}).Apply(batchResult.Findings)
		if minimumSeverityCandidates > 0 && len(batchResult.Findings) == 0 {
			e.deps.logger.Warn("게시 기준을 충족한 지적의 diff 근거를 확정하지 못했습니다",
				"target", input.target.Reference(),
				"batch", index+1,
				"provider", batchResponse.Provider,
				"candidates", minimumSeverityCandidates,
				"unknown_file", batchEvidenceReport.UnknownFile,
				"invalid_span", batchEvidenceReport.InvalidSpan,
				"non_added_span", batchEvidenceReport.NonAddedSpan,
				"not_found", batchEvidenceReport.NotFound,
				"ambiguous", batchEvidenceReport.Ambiguous)
		}
		unitFinishedAt := e.deps.clock.Now()
		if err := e.finishUnitCheckpoint(ctx, input.runID, input.runLease, unit.Hash, claim.LeaseToken, reviewworkflow.UnitResult{
			Status:         reviewworkflow.UnitStatusSucceeded,
			InputHash:      inputHash,
			Provider:       batchResponse.Provider,
			Model:          batchResponse.Model,
			MultipleModels: batchResponse.ModelLabel == "복수 모델",
			Usage:          batchResponse.Usage,
			Review:         batchResult,
			ToolExecutions: batchResponse.ToolExecutions,
			FinishedAt:     unitFinishedAt,
		}); err != nil {
			return fail("리뷰 unit 결과를 저장하지 못했습니다", err)
		}
		transitionCoverage(input.coverage, unit.Hash, reviewworkflow.CoverageStatusReviewed, &unitFinishedAt)
		e.deps.logger.Info("배치 리뷰 결과",
			"target", input.target.Reference(),
			"batch", index+1,
			"provider", batchResponse.Provider,
			"files", len(batch),
			"raw_findings", report.RawFindings,
			"schema_dropped", report.Dropped,
			"evidence_exact", batchEvidenceReport.Exact,
			"evidence_normalized", batchEvidenceReport.Normalized,
			"evidence_reanchored", batchEvidenceReport.Reanchored,
			"evidence_unknown_file", batchEvidenceReport.UnknownFile,
			"evidence_invalid_span", batchEvidenceReport.InvalidSpan,
			"evidence_non_added_span", batchEvidenceReport.NonAddedSpan,
			"evidence_not_found", batchEvidenceReport.NotFound,
			"evidence_ambiguous", batchEvidenceReport.Ambiguous,
			"has_summary", report.HasSummary,
			"content_length", contentLength)
		if report.Dropped > 0 {
			e.deps.logger.Warn("형식이 맞지 않아 버린 지적이 있습니다", "target", input.target.Reference(), "dropped", report.Dropped)
		}
		batchResults = append(batchResults, batchResult)
		result.reviewed = append(result.reviewed, batch...)
	}
	result.review, result.reduced = (reviewanalysis.ResultReducer{}).Reduce(batchResults)
	captureResponse()
	return result, nil
}

func (e *reviewUnitExecutor) toolExecutor(target pullrequest.Target, maxExtraReads int, paths *promptPathMap) outbound.ToolExecutor {
	if maxExtraReads <= 0 || e.deps.tools == nil {
		return nil
	}
	return &pathMappingToolExecutor{
		delegate:       e.deps.tools.ForTarget(target, target.HeadSHA, maxExtraReads),
		paths:          paths,
		mask:           e.deps.masker.Mask,
		remainingChars: 2000,
	}
}
