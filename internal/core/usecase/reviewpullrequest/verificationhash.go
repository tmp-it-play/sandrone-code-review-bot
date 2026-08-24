package reviewpullrequest

import (
	"errors"
	"slices"

	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

type verificationHashInput struct {
	Version                string
	Messages               []llm.Message
	Policy                 llm.PolicyHashInputs
	CandidateOccurrenceIDs []string
	Temperature            float64
	MaxOutputTokens        int
}

func verificationInputHash(request llm.Request, policy llm.PolicyHashInputs, candidates []review.Finding) (string, []string, error) {
	identifiers, err := verificationOccurrenceIDs(candidates)
	if err != nil {
		return "", nil, err
	}
	hash, err := hashJSON(verificationHashInput{
		Version:                "finding-verification-input-v1",
		Messages:               request.Messages,
		Policy:                 policy,
		CandidateOccurrenceIDs: identifiers,
		Temperature:            request.Temperature,
		MaxOutputTokens:        request.MaxOutputTokens,
	})
	if err != nil {
		return "", nil, err
	}
	return hash, identifiers, nil
}

func verificationOccurrenceIDs(findings []review.Finding) ([]string, error) {
	identifiers := make([]string, 0, len(findings))
	seen := make(map[string]struct{}, len(findings))
	for _, finding := range findings {
		identifier := finding.OccurrenceID.String()
		if identifier == "" {
			identifier = review.NewOccurrenceFingerprint(finding).String()
		}
		if identifier == "" {
			return nil, errors.New("finding verifier 후보 occurrence ID가 없습니다")
		}
		if _, exists := seen[identifier]; exists {
			return nil, errors.New("finding verifier 후보 occurrence ID가 중복되었습니다")
		}
		seen[identifier] = struct{}{}
		identifiers = append(identifiers, identifier)
	}
	slices.Sort(identifiers)
	return identifiers, nil
}

func findingsFromVerificationCheckpoint(findings []review.Finding, candidateIDs []string, supportedIDs []string) ([]review.Finding, error) {
	candidates := make(map[string]struct{}, len(candidateIDs))
	for _, identifier := range candidateIDs {
		candidates[identifier] = struct{}{}
	}
	supported := make(map[string]struct{}, len(supportedIDs))
	for _, identifier := range supportedIDs {
		if _, exists := candidates[identifier]; !exists {
			return nil, errors.New("finding verifier checkpoint가 현재 후보와 일치하지 않습니다")
		}
		if _, exists := supported[identifier]; exists {
			return nil, errors.New("finding verifier checkpoint occurrence ID가 중복되었습니다")
		}
		supported[identifier] = struct{}{}
	}
	verified := make([]review.Finding, 0, len(supported))
	for _, finding := range findings {
		identifier := finding.OccurrenceID.String()
		if identifier == "" {
			identifier = review.NewOccurrenceFingerprint(finding).String()
		}
		if _, exists := supported[identifier]; exists {
			verified = append(verified, finding)
		}
	}
	if len(verified) != len(supported) {
		return nil, errors.New("finding verifier checkpoint를 후보에 적용하지 못했습니다")
	}
	return verified, nil
}
