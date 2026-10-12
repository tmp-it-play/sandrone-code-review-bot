package reviewpullrequest

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
	"github.com/it-play/sandrone-code-review-bot/internal/core/port/outbound"
	"github.com/it-play/sandrone-code-review-bot/internal/core/prompt"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewsummary"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

const summaryCallReserve = 2
const summaryTimeout = 35 * time.Second

type reviewSummaryReducer struct {
	repository outbound.ReviewSummaryRepository
	completer  reviewVerificationCompleter
	masker     outbound.Masker
}

func (r *reviewSummaryReducer) reduce(ctx context.Context, input reviewResultFinalization) (reviewworkflow.SummaryCheckpoint, error) {
	request := input.request
	request.TaskRole = llm.TaskRoleReducer
	request.Messages = llm.MaskMessages((prompt.ReviewSummaryPrompt{Overviews: input.execution.overviews, Config: input.config}).Messages(), r.masker.Mask)
	request.MaxOutputTokens = 2048
	request.RequiredOutputTokens = 1024
	request.Tools = nil
	request.ForceJSON = true
	request.RequireCompletePrompt = true
	request.ResponseSchema = prompt.ReviewSummaryOutputSchema()
	request.ResponseValidation = llm.ResponseValidation{Validator: reviewsummary.Validate}
	inputHash, err := hashJSON(struct {
		Version  string
		Messages []llm.Message
		Policy   llm.PolicyHashInputs
	}{"review-overview-v1", request.Messages, r.completer.PolicyHashInputs(request)})
	if err != nil {
		return reviewworkflow.SummaryCheckpoint{}, err
	}
	checkpoint, found, err := r.repository.ReviewSummary(ctx, input.runID, input.runLease)
	if err != nil {
		return checkpoint, err
	}
	if found {
		if checkpoint.InputHash != inputHash {
			return checkpoint, reviewworkflow.ErrPlanChanged
		}
		return checkpoint, nil
	}
	checkpoint.InputHash = inputHash
	checkpoint.Overview = "변경 내용 요약을 제공하지 못했습니다. 검토 결과는 아래에 표시합니다."
	checkpoint.Fallback = true
	if !strings.HasPrefix(strings.ToLower(input.config.Language), "ko") {
		checkpoint.Overview = "A change summary is unavailable. Review results are shown below."
	}
	if len(input.execution.overviews) == 1 {
		if overview, err := reviewsummary.CanonicalOverview(input.execution.overviews[0]); err == nil {
			checkpoint.Overview = overview
			checkpoint.Fallback = false
		}
	}
	parentBudget := request.ExternalCallBudget
	if parentBudget == nil {
		return checkpoint, errors.New("최종 요약 호출 예산이 없습니다")
	}
	request.ExternalCallBudget = llm.NewScopedDurableExternalCallBudget(parentBudget.Limit(), parentBudget.Used(), min(parentBudget.Used()+summaryCallReserve, parentBudget.Limit()), parentBudget.Reserve)
	sourceLength := 0
	for _, overview := range input.execution.overviews {
		sourceLength += utf8.RuneCountInString(overview)
	}
	if checkpoint.Fallback && len(input.execution.overviews) > 0 && sourceLength <= 20000 && request.ExternalCallBudget.Remaining() > 0 {
		completionContext, cancel := context.WithTimeout(ctx, summaryTimeout)
		response, completeErr := r.completer.Complete(completionContext, request, nil)
		cancel()
		response.Content = r.masker.Mask(response.Content)
		if completeErr == nil && response.Completed() {
			if overview, parseErr := reviewsummary.Parse(response.Content); parseErr == nil {
				checkpoint.Overview = overview
				checkpoint.Fallback = false
			}
		}
		response.Content = ""
		response.ToolCalls = nil
		checkpoint.Response = response
		if errors.Is(completeErr, llm.ErrExternalCallBudgetUnavailable) {
			return checkpoint, completeErr
		}
	}
	if ctx.Err() != nil {
		return checkpoint, ctx.Err()
	}
	checkpointContext, cancel := durableCheckpointContext(ctx)
	defer cancel()
	if err := r.repository.SaveReviewSummary(checkpointContext, input.runID, input.runLease, checkpoint); err != nil {
		return checkpoint, fmt.Errorf("최종 요약 결과를 저장하지 못했습니다: %w", err)
	}
	return checkpoint, nil
}
