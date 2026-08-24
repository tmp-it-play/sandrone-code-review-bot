package parsing

import (
	"encoding/json"
	"strings"

	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

type resultPayload struct {
	Summary struct {
		Overview string `json:"overview"`
		Files    []struct {
			Path string `json:"path"`
			Note string `json:"note"`
		} `json:"files"`
	} `json:"summary"`
	Findings []json.RawMessage `json:"findings"`
}

func (p resultPayload) toDomain() (review.Result, int) {
	result := review.Result{}
	dropped := 0
	result.Summary.Overview = strings.TrimSpace(p.Summary.Overview)
	for _, file := range p.Summary.Files {
		path := strings.TrimSpace(file.Path)
		if path == "" {
			continue
		}
		result.Summary.Files = append(result.Summary.Files, review.FileNote{Path: path, Note: strings.TrimSpace(file.Note)})
	}
	for _, raw := range p.Findings {
		entry := findingPayload{}
		if err := json.Unmarshal(raw, &entry); err != nil {
			dropped++
			continue
		}
		finding, valid := entry.toDomain()
		if !valid {
			dropped++
			continue
		}
		result.Findings = append(result.Findings, finding)
	}
	return result, dropped
}
