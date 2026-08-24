package reviewpullrequest

import (
	"context"

	"github.com/it-play/sandrone-code-review-bot/internal/core/dedupe"
	"github.com/it-play/sandrone-code-review-bot/internal/core/mapping"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewanalysis"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
	"github.com/it-play/sandrone-code-review-bot/internal/core/selection"
)

type reviewResultFinalizer struct {
	deps reviewResultFinalizerDependencies
}

func newReviewResultFinalizer(deps reviewResultFinalizerDependencies) *reviewResultFinalizer {
	return &reviewResultFinalizer{deps: deps}
}

func (f *reviewResultFinalizer) finalize(ctx context.Context, input reviewResultFinalization) (reviewResultFinalizationResult, error) {
	result := reviewResultFinalizationResult{
		failureResponse: input.execution.response,
		runStatus:       input.runStatus,
	}
	response := input.execution.response
	usage := response.Usage
	toolExecutions := response.ToolExecutions
	modelsUsed := input.execution.modelsUsed
	multipleModels := input.execution.multipleModels
	captureFailureResponse := func() {
		response.Usage = usage
		response.ToolExecutions = toolExecutions
		if multipleModels || len(modelsUsed) > 1 {
			response.Provider = "multiple"
			response.Model = "multiple"
			response.ModelLabel = "복수 모델"
		}
		result.failureResponse = response
	}

	findings := selection.SeverityFilter{Minimum: input.config.MinSeverity}.Apply(input.execution.review.Findings)
	afterSeverity := len(findings)
	var finalEvidenceReport reviewanalysis.EvidenceReport
	findings, finalEvidenceReport = (reviewanalysis.EvidenceVerifier{}).VerifyWithReport(findings, input.execution.reviewed)
	known, err := f.deps.findings.Fingerprints(ctx, input.target)
	if err != nil {
		result.errorMessage = "기존 지적을 읽지 못했습니다"
		return result, err
	}
	findings, duplicates := dedupe.DuplicateFilter{Known: known}.Apply(findings)
	findings, omittedFindings := publishedFindingBudget(findings)
	verification, err := f.deps.verifier.verify(ctx, findingVerification{
		runID:             input.runID,
		runLease:          input.runLease,
		findings:          findings,
		files:             input.execution.reviewed,
		request:           input.request,
		reviewerProviders: input.execution.reviewerProviders,
		paths:             input.paths,
	})
	usage = usage.Add(verification.response.Usage)
	toolExecutions += verification.response.ToolExecutions
	trackReviewModel(modelsUsed, verification.response.Provider, verification.response.Model)
	multipleModels = multipleModels || verification.response.ModelLabel == "복수 모델"
	if verification.response.Provider != "" || verification.response.Model != "" {
		response.Provider = verification.response.Provider
		response.Model = verification.response.Model
		response.ModelLabel = verification.response.ModelLabel
	}
	captureFailureResponse()
	if err != nil {
		result.errorMessage = "독립 finding verifier 결과를 저장하지 못했습니다"
		return result, err
	}
	findings = verification.findings
	if verification.unavailable {
		result.runStatus = reviewworkflow.RunStatusPartial
	}
	var finalReduced int
	findings, finalReduced = (dedupe.RootCauseReducer{}).Apply(findings)
	findings = mapping.PositionMapper{MaxInline: input.config.MaxInlineComments}.Map(findings, input.execution.reviewed)
	f.deps.logger.Info("지적 집계",
		"target", input.target.Reference(),
		"collected", len(input.execution.review.Findings),
		"after_severity", afterSeverity,
		"schema_dropped", input.execution.schemaDropped,
		"checkpoint_reused_findings", input.execution.reusedFindings,
		"evidence_exact", input.execution.evidenceReport.Exact,
		"evidence_normalized", input.execution.evidenceReport.Normalized,
		"evidence_reanchored", input.execution.evidenceReport.Reanchored,
		"evidence_unknown_file", input.execution.evidenceReport.UnknownFile,
		"evidence_invalid_span", input.execution.evidenceReport.InvalidSpan,
		"evidence_non_added_span", input.execution.evidenceReport.NonAddedSpan,
		"evidence_not_found", input.execution.evidenceReport.NotFound,
		"evidence_ambiguous", input.execution.evidenceReport.Ambiguous,
		"evidence_accepted", input.execution.evidenceReport.Accepted(),
		"evidence_rejected", input.execution.evidenceReport.Dropped(),
		"final_evidence_exact", finalEvidenceReport.Exact,
		"final_evidence_normalized", finalEvidenceReport.Normalized,
		"final_evidence_reanchored", finalEvidenceReport.Reanchored,
		"final_evidence_rejected", finalEvidenceReport.Dropped(),
		"root_causes_collapsed", input.execution.reduced+finalReduced,
		"verifier_used", verification.used,
		"verifier_rejected", verification.rejected,
		"verification_unavailable", verification.unavailable,
		"omitted_by_budget", omittedFindings,
		"duplicates", duplicates,
		"final", len(findings),
		"min_severity", string(input.config.MinSeverity))

	draft := review.Result{Summary: input.execution.review.Summary, Findings: findings}
	response.Usage = usage
	response.ToolExecutions = toolExecutions
	if multipleModels || len(modelsUsed) > 1 {
		response.Provider = "multiple"
		response.Model = "multiple"
		response.ModelLabel = "복수 모델"
	}
	attribution := attributionOf(response)
	style := review.Style{Emoji: input.config.Emoji, Tone: string(input.config.Tone)}
	view := review.SummaryView{
		Summary:         draft.Summary,
		Fallback:        draft.Fallback(),
		InlineCount:     len(draft.Inline()),
		Attribution:     attribution,
		Style:           style,
		Trigger:         input.trigger,
		Incremental:     input.incremental,
		OmittedFindings: omittedFindings,
	}
	result.draft = draft
	result.view = view
	result.attribution = attribution
	result.style = style
	result.response = response
	result.failureResponse = response
	result.verificationUnavailable = verification.unavailable
	return result, nil
}
