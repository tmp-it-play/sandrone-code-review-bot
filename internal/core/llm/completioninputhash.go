package llm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

type completionInputHashPayload struct {
	Version               string
	Messages              []Message
	Temperature           float64
	MaxOutputTokens       int
	Tools                 []Tool
	ForceJSON             bool
	Providers             []string
	ExcludedProviders     []string
	TaskRole              TaskRole
	DataClassification    DataClassification
	RequireCompletePrompt bool
	ResponseValidation    ResponseValidation
	Policy                PolicyHashInputs
	ResultPolicy          string
}

func CompletionInputHash(request Request, policy PolicyHashInputs, resultPolicy string) (string, error) {
	encoded, err := json.Marshal(completionInputHashPayload{
		Version:               "publication-completion-input-v2",
		Messages:              request.Messages,
		Temperature:           request.Temperature,
		MaxOutputTokens:       request.MaxOutputTokens,
		Tools:                 request.Tools,
		ForceJSON:             request.ForceJSON,
		Providers:             request.Providers,
		ExcludedProviders:     request.ExcludedProviders,
		TaskRole:              request.TaskRole,
		DataClassification:    request.DataClassification,
		RequireCompletePrompt: request.RequireCompletePrompt,
		ResponseValidation:    request.ResponseValidation,
		Policy:                policy,
		ResultPolicy:          resultPolicy,
	})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}
