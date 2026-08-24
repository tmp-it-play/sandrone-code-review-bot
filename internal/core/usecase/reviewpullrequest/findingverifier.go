package reviewpullrequest

import (
	"context"
	"fmt"
	"sort"

	"github.com/it-play/sandrone-code-review-bot/internal/core/findingverification"
	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

func (u *UseCase) verifyFindings(ctx context.Context, runID uint64, runLease string, findings []review.Finding, files []pullrequest.ChangedFile, baseRequest llm.Request, reviewerProviders map[string]struct{}, paths *promptPathMap) ([]review.Finding, llm.Response, int, bool, bool, error) {
	excluded := make([]string, 0, len(reviewerProviders))
	for name := range reviewerProviders {
		excluded = append(excluded, name)
	}
	sort.Strings(excluded)
	request := baseRequest
	request.Messages = llm.MaskMessages((findingverification.PromptBuilder{}).Messages(paths.PromptFindings(findings), paths.PromptFiles(files)), u.deps.Masker.Mask)
	request.TaskRole = llm.TaskRoleVerifier
	request.ExcludedProviders = excluded
	request.MaxOutputTokens = 4096
	request.ForceJSON = true
	request.ResponseValidation = llm.ResponseValidation{}
	policy := u.deps.Completer.PolicyHashInputs(request)
	inputHash, candidateIDs, err := verificationInputHash(request, policy, findings)
	if err != nil {
		return nil, llm.Response{}, len(findings), false, true, fmt.Errorf("finding verifier 입력 hash를 만들지 못했습니다: %w", err)
	}
	checkpoint, checkpointFound, err := u.deps.Workflows.ReviewVerification(ctx, runID, runLease, inputHash)
	if err != nil {
		return nil, llm.Response{}, len(findings), false, true, err
	}
	checkpointResponse := checkpoint.MetadataResponse()
	if len(findings) == 0 {
		return findings, checkpointResponse, 0, false, false, nil
	}
	if checkpointFound {
		if checkpoint.RunID != runID || checkpoint.InputHash != inputHash {
			checkpointFound = false
			u.deps.Logger.Warn("저장된 finding verifier 결과의 실행 식별자가 일치하지 않아 다시 검증합니다", "run", runID)
		}
	}
	if checkpointFound {
		supported, checkpointErr := findingsFromVerificationCheckpoint(findings, candidateIDs, checkpoint.SupportedOccurrenceIDs)
		if checkpointErr == nil {
			u.deps.Logger.Info("저장된 finding verifier 결과를 재사용했습니다", "run", runID)
			return supported, checkpointResponse, len(findings) - len(supported), true, false, nil
		}
		u.deps.Logger.Warn("저장된 finding verifier 결과가 현재 후보와 일치하지 않아 다시 검증합니다", "run", runID, "error", checkpointErr)
	}
	if len(policy.EligibleProviders) == 0 {
		u.deps.Logger.Info("독립 finding verifier로 사용할 다른 프로바이더가 없어 결정론적 검증 결과를 사용합니다")
		return findings, checkpointResponse, 0, false, false, nil
	}
	if request.ExternalCallBudget == nil || request.ExternalCallBudget.Remaining() == 0 {
		u.deps.Logger.Warn("독립 finding verifier 호출 예산이 없어 결정론적 검증 결과를 사용합니다")
		return findings, checkpointResponse, 0, false, true, nil
	}
	response, err := u.deps.Completer.Complete(ctx, request, nil)
	response.Content = u.deps.Masker.Mask(response.Content)
	if err != nil {
		u.deps.Logger.Warn("독립 finding verifier를 완료하지 못해 결정론적 검증 결과를 사용합니다", "error", u.deps.Masker.Mask(err.Error()))
		aggregate, recordErr := u.recordVerificationAttempt(ctx, runID, runLease, inputHash, response)
		return findings, aggregate, 0, true, true, recordErr
	}
	if !response.Completed() {
		u.deps.Logger.Warn("독립 finding verifier 응답이 완료되지 않아 결정론적 검증 결과를 사용합니다", "finish_reason", response.FinishReason)
		aggregate, recordErr := u.recordVerificationAttempt(ctx, runID, runLease, inputHash, response)
		return findings, aggregate, 0, true, true, recordErr
	}
	decisions, err := (findingverification.Parser{}).Parse(response.Content, findings)
	if err != nil {
		u.deps.Logger.Warn("독립 finding verifier 응답이 유효하지 않아 결정론적 검증 결과를 사용합니다", "error", err)
		aggregate, recordErr := u.recordVerificationAttempt(ctx, runID, runLease, inputHash, response)
		return findings, aggregate, 0, true, true, recordErr
	}
	supported := decisions.Supported(findings)
	supportedIDs, err := verificationOccurrenceIDs(supported)
	if err != nil {
		return nil, response, len(findings), true, true, fmt.Errorf("finding verifier 결과 ID를 만들지 못했습니다: %w", err)
	}
	checkpoint = reviewworkflow.VerificationCheckpoint{
		RunID:                  runID,
		InputHash:              inputHash,
		SupportedOccurrenceIDs: supportedIDs,
		Provider:               response.Provider,
		Model:                  response.Model,
		MultipleModels:         response.ModelLabel == "복수 모델" || response.Provider == "multiple" || response.Model == "multiple",
		Usage:                  response.Usage,
		ToolExecutions:         response.ToolExecutions,
		Completed:              true,
		FinishedAt:             u.deps.Clock.Now(),
	}
	checkpointContext, checkpointCancel := durableCheckpointContext(ctx)
	checkpoint, err = u.deps.Workflows.SaveReviewVerification(checkpointContext, runID, runLease, checkpoint)
	checkpointCancel()
	if err != nil {
		return nil, response, len(findings), true, true, err
	}
	return supported, checkpoint.MetadataResponse(), len(findings) - len(supported), true, false, nil
}

func (u *UseCase) recordVerificationAttempt(ctx context.Context, runID uint64, runLease string, inputHash string, response llm.Response) (llm.Response, error) {
	checkpointContext, checkpointCancel := durableCheckpointContext(ctx)
	defer checkpointCancel()
	checkpoint, err := u.deps.Workflows.RecordReviewVerificationAttempt(checkpointContext, runID, runLease, inputHash, response, u.deps.Clock.Now())
	if err != nil {
		return response, err
	}
	return checkpoint.MetadataResponse(), nil
}
