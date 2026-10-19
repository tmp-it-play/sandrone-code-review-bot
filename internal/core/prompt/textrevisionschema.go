package prompt

import "github.com/it-play/sandrone-code-review-bot/internal/core/llm"

func TextRevisionOutputSchema() *llm.ResponseSchema {
	return &llm.ResponseSchema{Name: "publication_text_revision_v1", Schema: map[string]any{
		"type": "object",
		"properties": map[string]any{"edits": map[string]any{
			"type": "array", "items": map[string]any{
				"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string"}, "text": map[string]any{"type": "string"}},
				"required": []string{"id", "text"}, "additionalProperties": false,
			},
		}},
		"required": []string{"edits"}, "additionalProperties": false,
	}}
}
