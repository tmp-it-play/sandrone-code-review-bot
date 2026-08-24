package findingverification

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

type Parser struct{}

func (Parser) Parse(raw string, candidates []review.Finding) (DecisionSet, error) {
	expected, err := expectedOccurrences(candidates)
	if err != nil {
		return DecisionSet{}, err
	}

	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	var payload responseEnvelope
	if err := decoder.Decode(&payload); err != nil {
		return DecisionSet{}, fmt.Errorf("%w: %v", ErrInvalidResponse, err)
	}
	if payload.Decisions == nil {
		return DecisionSet{}, ErrInvalidResponse
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return DecisionSet{}, ErrInvalidResponse
		}
		return DecisionSet{}, fmt.Errorf("%w: %v", ErrInvalidResponse, err)
	}

	entries := make(map[string]Decision, len(payload.Decisions))
	for _, entry := range payload.Decisions {
		if _, exists := expected[entry.OccurrenceID]; !exists {
			return DecisionSet{}, fmt.Errorf("%w: %s", ErrUnknownOccurrenceID, entry.OccurrenceID)
		}
		if _, exists := entries[entry.OccurrenceID]; exists {
			return DecisionSet{}, fmt.Errorf("%w: %s", ErrDuplicateOccurrenceID, entry.OccurrenceID)
		}
		status := Status(entry.Status)
		if !status.Valid() {
			return DecisionSet{}, fmt.Errorf("%w: %s", ErrInvalidStatus, entry.Status)
		}
		reason := strings.TrimSpace(entry.Reason)
		if reason == "" {
			return DecisionSet{}, fmt.Errorf("%w: %s", ErrMissingReason, entry.OccurrenceID)
		}
		entries[entry.OccurrenceID] = Decision{
			OccurrenceID: review.Fingerprint(entry.OccurrenceID),
			Status:       status,
			Reason:       reason,
		}
	}
	for occurrenceID := range expected {
		if _, exists := entries[occurrenceID]; !exists {
			return DecisionSet{}, fmt.Errorf("%w: %s", ErrMissingOccurrenceID, occurrenceID)
		}
	}
	return DecisionSet{entries: entries}, nil
}

func expectedOccurrences(candidates []review.Finding) (map[string]struct{}, error) {
	expected := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		identifier := occurrenceID(candidate).String()
		if identifier == "" {
			return nil, ErrInvalidCandidates
		}
		if _, exists := expected[identifier]; exists {
			return nil, fmt.Errorf("%w: %s", ErrDuplicateOccurrenceID, identifier)
		}
		expected[identifier] = struct{}{}
	}
	return expected, nil
}
