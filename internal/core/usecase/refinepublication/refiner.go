package refinepublication

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"
	"unicode/utf8"

	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
	"github.com/it-play/sandrone-code-review-bot/internal/core/port/outbound"
	"github.com/it-play/sandrone-code-review-bot/internal/core/prompt"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"github.com/it-play/sandrone-code-review-bot/internal/core/setting"
	"github.com/it-play/sandrone-code-review-bot/internal/core/textrevision"
)

const CallReserve = 2
const maxCalls = 4
const timeout = 45 * time.Second

type Refiner struct {
	completer outbound.Completer
	masker    outbound.Masker
	logger    *slog.Logger
}

func New(completer outbound.Completer, masker outbound.Masker, logger *slog.Logger) *Refiner {
	return &Refiner{completer: completer, masker: masker, logger: logger}
}

func (r *Refiner) Plan(source review.Result, config setting.RepoConfig, request llm.Request, compact bool) (Plan, error) {
	plan := Plan{Document: textrevision.NewDocument(source, compact), Config: config}
	request.TaskRole = llm.TaskRoleEditor
	request.ExcludedProviders = nil
	request.Tools = nil
	request.Temperature = min(config.Temperature, 0.2)
	request.MaxOutputTokens = config.MaxOutputTokens
	request.RequiredOutputTokens = 0
	request.ForceJSON = true
	request.RequireCompletePrompt = true
	request.FailFastOnIncomplete = true
	request.ResponseSchema = prompt.TextRevisionOutputSchema()
	request.ResponseValidation = llm.ResponseValidation{Policy: llm.ResponseValidationNonEmpty}
	request.Messages = r.messages(plan, plan.Document.Fields)
	sourceJSON, err := json.Marshal(source)
	if err != nil {
		return plan, err
	}
	plan.Request = request
	plan.InputHash, err = llm.CompletionInputHash(request, r.completer.PolicyHashInputs(request), "publication-text-revision-v1:"+string(sourceJSON))
	return plan, err
}

func (r *Refiner) Refine(ctx context.Context, plan Plan) textrevision.Checkpoint {
	checkpoint := textrevision.Checkpoint{InputHash: plan.InputHash}
	parentBudget := plan.Request.ExternalCallBudget
	if parentBudget == nil || parentBudget.Remaining() == 0 {
		checkpoint.Fallback = true
		return checkpoint
	}
	request := plan.Request
	request.ExternalCallBudget = llm.NewScopedDurableExternalCallBudget(parentBudget.Limit(), parentBudget.Used(), min(parentBudget.Used()+maxCalls, parentBudget.Used()+parentBudget.Remaining()), parentBudget.Reserve)
	completionContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	remaining := plan.Document.Fields
	for len(remaining) > 0 {
		if completionContext.Err() != nil || request.ExternalCallBudget.Remaining() == 0 {
			checkpoint.Fallback = true
			break
		}
		batch, rest := r.batch(plan, remaining)
		remaining = rest
		if len(batch) == 0 {
			checkpoint.Fallback = true
			continue
		}
		request.Messages = r.messages(plan, batch)
		request.RequiredOutputTokens = min(request.MaxOutputTokens, 256+2*fieldCharacters(batch))
		request.ResponseValidation.Validator = func(content string) error {
			_, err := textrevision.Parse(content, batch)
			return err
		}
		response, err := r.completer.Complete(completionContext, request, nil)
		checkpoint.Response = llm.MergeResponseMetadata(checkpoint.Response, response)
		if err != nil || !response.Completed() || len(response.ToolCalls) > 0 {
			checkpoint.Fallback = true
			r.logger.Info("문체 보정을 완료하지 못해 원문을 유지합니다")
			continue
		}
		edits, err := textrevision.Parse(r.masker.Mask(response.Content), batch)
		if err != nil {
			checkpoint.Fallback = true
			r.logger.Info("문체 보정 응답이 유효하지 않아 원문을 유지합니다", "error", err)
			continue
		}
		candidate := checkpoint
		candidate.Edits = append(append([]textrevision.Edit{}, checkpoint.Edits...), edits...)
		candidate.Fallback = true
		if _, err := plan.Document.Apply(candidate); err != nil {
			checkpoint.Fallback = true
			continue
		}
		checkpoint.Edits = candidate.Edits
	}
	return checkpoint
}

func (r *Refiner) batch(plan Plan, fields []textrevision.Field) ([]textrevision.Field, []textrevision.Field) {
	budget := min(plan.Config.MaxPromptChars, r.completer.PromptBudgetFor(plan.Request))
	characterLimit := max(0, (plan.Request.MaxOutputTokens-256)/2)
	count := 0
	for count < len(fields) {
		candidate := fields[:count+1]
		if fieldCharacters(candidate) > characterLimit || llm.MessagesSize(r.messages(plan, candidate)) > budget {
			break
		}
		count++
	}
	if count == 0 {
		return nil, fields[1:]
	}
	return fields[:count], fields[count:]
}

func (r *Refiner) messages(plan Plan, fields []textrevision.Field) []llm.Message {
	return llm.MaskMessages((prompt.TextRevisionPrompt{Fields: fields, Config: plan.Config, Compact: plan.Document.Compact}).Messages(), r.masker.Mask)
}

func fieldCharacters(fields []textrevision.Field) int {
	characters := 0
	for _, field := range fields {
		characters += utf8.RuneCountInString(field.Text) + len(field.ID) + 32
	}
	return characters
}
