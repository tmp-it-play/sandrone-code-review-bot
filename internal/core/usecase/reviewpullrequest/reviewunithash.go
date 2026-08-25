package reviewpullrequest

import (
	"sort"

	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
)

type reviewUnitHashInput struct {
	Version              string
	RepositoryScope      string
	ConfigHash           string
	UnitHash             string
	Messages             []llm.Message
	Temperature          float64
	MaxOutputTokens      int
	RequiredOutputTokens int
	Providers            []string
	Policy               llm.PolicyHashInputs
	MaxExtraReads        int
	Tools                []llm.Tool
	ToolContract         string
	ResultPolicy         string
	Validation           llm.ResponseValidation
	FailFastOnIncomplete bool
}

func reviewUnitInputHash(repositoryScope string, configHash string, unitHash string, request llm.Request, policy llm.PolicyHashInputs, maxExtraReads int, tools []llm.Tool) (string, error) {
	definitions := append([]llm.Tool(nil), tools...)
	sort.Slice(definitions, func(left int, right int) bool {
		return definitions[left].Name < definitions[right].Name
	})
	return hashJSON(reviewUnitHashInput{
		Version:              "review-unit-input-v7",
		RepositoryScope:      repositoryScope,
		ConfigHash:           configHash,
		UnitHash:             unitHash,
		Messages:             request.Messages,
		Temperature:          request.Temperature,
		MaxOutputTokens:      request.MaxOutputTokens,
		RequiredOutputTokens: request.RequiredOutputTokens,
		Providers:            request.Providers,
		Policy:               policy,
		MaxExtraReads:        maxExtraReads,
		Tools:                definitions,
		ToolContract:         "tool-contract-v1",
		ResultPolicy:         "review-result-policy-v5",
		Validation:           request.ResponseValidation,
		FailFastOnIncomplete: request.FailFastOnIncomplete,
	})
}
