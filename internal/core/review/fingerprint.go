package review

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
)

type Fingerprint string

func NewFingerprint(finding Finding) Fingerprint {
	return NewRootCauseFingerprint(finding)
}

func NewRootCauseFingerprint(finding Finding) Fingerprint {
	rootCause := normalizeForFingerprint(finding.RootCause)
	if rootCause != "" {
		return fingerprintOf(rootCause)
	}
	return fingerprintOf(strings.Join([]string{
		normalizeForFingerprint(finding.Title),
		normalizeForFingerprint(finding.Body),
	}, "\x1f"))
}

func NewOccurrenceFingerprint(finding Finding) Fingerprint {
	return NewAnchoredOccurrenceFingerprint(
		NewRootCauseFingerprint(finding),
		finding.File,
		finding.SpanStart(),
		finding.SpanEnd(),
		finding.Evidence,
	)
}

func NewAnchoredOccurrenceFingerprint(root Fingerprint, path string, line int, endLine int, evidence string) Fingerprint {
	return fingerprintOf(strings.Join([]string{
		root.String(),
		strings.TrimSpace(path),
		strconv.Itoa(line),
		strconv.Itoa(endLine),
		evidence,
	}, "\x1f"))
}

func (f Fingerprint) String() string {
	return string(f)
}

func fingerprintOf(seed string) Fingerprint {
	sum := sha256.Sum256([]byte(seed))
	return Fingerprint(hex.EncodeToString(sum[:]))
}

func normalizeForFingerprint(text string) string {
	return strings.Join(strings.Fields(strings.ToLower(text)), " ")
}
