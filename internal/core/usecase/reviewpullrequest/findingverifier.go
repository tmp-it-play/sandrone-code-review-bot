package reviewpullrequest

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/it-play/sandrone-code-review-bot/internal/core/findingverification"
	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

type findingVerifier struct {
	deps findingVerifierDependencies
}

func newFindingVerifier(deps findingVerifierDependencies) *findingVerifier {
	return &findingVerifier{deps: deps}
}

func (v *findingVerifier) verify(ctx context.Context, input findingVerification) (findingVerificationResult, error) {
	result := findingVerificationResult{}
	excluded := make([]string, 0, len(input.reviewerProviders))
	for name := range input.reviewerProviders {
		excluded = append(excluded, name)
	}
	sort.Strings(excluded)
	request := input.request
	request.Messages = llm.MaskMessages((findingverification.PromptBuilder{}).Messages(input.paths.PromptFindings(input.findings), input.paths.PromptFiles(input.files)), v.deps.masker.Mask)
	request.TaskRole = llm.TaskRoleVerifier
	request.ExcludedProviders = excluded
	request.MaxOutputTokens = 4096
	request.ForceJSON = true
	request.ResponseValidation = llm.ResponseValidation{}
	policy := v.deps.completer.PolicyHashInputs(request)
	inputHash, candidateIDs, err := verificationInputHash(request, policy, input.findings)
	if err != nil {
		result.rejected = len(input.findings)
		return result, fmt.Errorf("finding verifier 입력 hash를 만들지 못했습니다: %w", err)
	}
	checkpoint, checkpointFound, err := v.deps.verification.ReviewVerification(ctx, input.runID, input.runLease, inputHash)
	if err != nil {
		result.rejected = len(input.findings)
		return result, fmt.Errorf("저장된 finding verifier 결과를 읽지 못했습니다: %w", err)
	}
	checkpointResponse := checkpoint.MetadataResponse()
	if len(input.findings) == 0 {
		result.findings = input.findings
		result.response = checkpointResponse
		return result, nil
	}
	if checkpointFound && (checkpoint.RunID != input.runID || checkpoint.InputHash != inputHash) {
		checkpointFound = false
		v.deps.logger.Warn("저장된 finding verifier 결과의 실행 식별자가 일치하지 않아 다시 검증합니다", "run", input.runID)
	}
	if checkpointFound {
		supported, checkpointErr := findingsFromVerificationCheckpoint(input.findings, candidateIDs, checkpoint.SupportedOccurrenceIDs)
		if checkpointErr == nil {
			v.deps.logger.Info("저장된 finding verifier 결과를 재사용했습니다", "run", input.runID)
			result.findings = supported
			result.response = checkpointResponse
			result.rejected = len(input.findings) - len(supported)
			result.used = true
			return result, nil
		}
		v.deps.logger.Warn("저장된 finding verifier 결과가 현재 후보와 일치하지 않아 다시 검증합니다", "run", input.runID, "error", checkpointErr)
	}
	if len(policy.EligibleProviders) == 0 {
		v.deps.logger.Info("독립 finding verifier로 사용할 다른 프로바이더가 없어 결정론적 검증 결과를 사용합니다")
		result.findings = input.findings
		result.response = checkpointResponse
		return result, nil
	}
	if request.ExternalCallBudget == nil || request.ExternalCallBudget.Remaining() == 0 {
		v.deps.logger.Warn("독립 finding verifier 호출 예산이 없어 결정론적 검증 결과를 사용합니다")
		result.findings = input.findings
		result.response = checkpointResponse
		result.unavailable = true
		return result, nil
	}
	response, err := v.deps.completer.Complete(ctx, request, nil)
	response.Content = v.deps.masker.Mask(response.Content)
	if err != nil {
		if errors.Is(err, llm.ErrExternalCallBudgetUnavailable) {
			aggregate, recordErr := v.recordAttempt(ctx, input.runID, input.runLease, inputHash, response)
			result.findings = input.findings
			result.response = aggregate
			result.used = true
			if recordErr != nil {
				return result, errors.Join(err, fmt.Errorf("finding verifier 시도를 저장하지 못했습니다: %w", recordErr))
			}
			return result, err
		}
		v.deps.logger.Warn("독립 finding verifier를 완료하지 못해 결정론적 검증 결과를 사용합니다", "error", v.deps.masker.Mask(err.Error()))
		return v.unavailableResult(ctx, input, inputHash, response)
	}
	if !response.Completed() {
		v.deps.logger.Warn("독립 finding verifier 응답이 완료되지 않아 결정론적 검증 결과를 사용합니다", "finish_reason", response.FinishReason)
		return v.unavailableResult(ctx, input, inputHash, response)
	}
	decisions, err := (findingverification.Parser{}).Parse(response.Content, input.findings)
	if err != nil {
		v.deps.logger.Warn("독립 finding verifier 응답이 유효하지 않아 결정론적 검증 결과를 사용합니다", "error", err)
		return v.unavailableResult(ctx, input, inputHash, response)
	}
	supported := decisions.Supported(input.findings)
	supportedIDs, err := verificationOccurrenceIDs(supported)
	if err != nil {
		result.response = response
		result.rejected = len(input.findings)
		result.used = true
		return result, fmt.Errorf("finding verifier 결과 ID를 만들지 못했습니다: %w", err)
	}
	checkpoint = reviewworkflow.VerificationCheckpoint{
		RunID:                  input.runID,
		InputHash:              inputHash,
		SupportedOccurrenceIDs: supportedIDs,
		Provider:               response.Provider,
		Model:                  response.Model,
		MultipleModels:         response.ModelLabel == "복수 모델" || response.Provider == "multiple" || response.Model == "multiple",
		Usage:                  response.Usage,
		ToolExecutions:         response.ToolExecutions,
		Completed:              true,
		FinishedAt:             v.deps.clock.Now(),
	}
	checkpointContext, checkpointCancel := durableCheckpointContext(ctx)
	checkpoint, err = v.deps.verification.SaveReviewVerification(checkpointContext, input.runID, input.runLease, checkpoint)
	checkpointCancel()
	if err != nil {
		result.response = response
		result.rejected = len(input.findings)
		result.used = true
		return result, fmt.Errorf("finding verifier 결과를 저장하지 못했습니다: %w", err)
	}
	result.findings = supported
	result.response = checkpoint.MetadataResponse()
	result.rejected = len(input.findings) - len(supported)
	result.used = true
	return result, nil
}

func (v *findingVerifier) unavailableResult(ctx context.Context, input findingVerification, inputHash string, response llm.Response) (findingVerificationResult, error) {
	aggregate, err := v.recordAttempt(ctx, input.runID, input.runLease, inputHash, response)
	result := findingVerificationResult{
		findings: input.findings,
		response: aggregate,
		used:     true,
	}
	if err != nil {
		return result, fmt.Errorf("finding verifier 시도를 저장하지 못했습니다: %w", err)
	}
	result.unavailable = true
	return result, nil
}

func (v *findingVerifier) recordAttempt(ctx context.Context, runID uint64, runLease string, inputHash string, response llm.Response) (llm.Response, error) {
	checkpointContext, checkpointCancel := durableCheckpointContext(ctx)
	defer checkpointCancel()
	checkpoint, err := v.deps.verification.RecordReviewVerificationAttempt(checkpointContext, runID, runLease, inputHash, response, v.deps.clock.Now())
	if err != nil {
		return response, err
	}
	return checkpoint.MetadataResponse(), nil
}
