package prompt

import "github.com/it-play/sandrone-code-review-bot/internal/core/llm"

func ReviewOutputSchema(includeFileNotes bool) *llm.ResponseSchema {
	text := map[string]any{"type": "string"}
	integer := map[string]any{"type": "integer"}
	summaryProperties := map[string]any{"overview": text}
	summaryRequired := []string{"overview"}
	if includeFileNotes {
		summaryProperties["files"] = map[string]any{
			"type": "array",
			"items": map[string]any{
				"type":                 "object",
				"properties":           map[string]any{"path": text, "note": text},
				"required":             []string{"path", "note"},
				"additionalProperties": false,
			},
		}
		summaryRequired = append(summaryRequired, "files")
	}
	return &llm.ResponseSchema{
		Name: "review_result_v1",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"summary": map[string]any{
					"type":                 "object",
					"properties":           summaryProperties,
					"required":             summaryRequired,
					"additionalProperties": false,
				},
				"findings": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"file": text, "line": integer, "endLine": integer,
							"severity": map[string]any{"type": "string", "enum": []string{"critical", "major", "minor", "nit"}},
							"title":    text, "body": text, "suggestion": text, "evidence": text, "rootCause": text,
						},
						"required":             []string{"file", "line", "endLine", "severity", "title", "body", "suggestion", "evidence", "rootCause"},
						"additionalProperties": false,
					},
				},
			},
			"required":             []string{"summary", "findings"},
			"additionalProperties": false,
		},
	}
}
