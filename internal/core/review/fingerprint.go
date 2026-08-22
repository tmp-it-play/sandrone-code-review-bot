package review

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode"
)

type Fingerprint string

func NewFingerprint(finding Finding) Fingerprint {
	seed := strings.Join([]string{
		strings.ToLower(strings.TrimSpace(finding.File)),
		string(finding.Severity),
		normalizeForFingerprint(finding.Title),
		normalizeForFingerprint(finding.Body),
	}, "\x1f")
	sum := sha256.Sum256([]byte(seed))
	return Fingerprint(hex.EncodeToString(sum[:]))
}

func (f Fingerprint) String() string {
	return string(f)
}

func normalizeForFingerprint(text string) string {
	var builder strings.Builder
	previousSpace := false
	for _, symbol := range strings.ToLower(text) {
		switch {
		case unicode.IsLetter(symbol) || unicode.IsDigit(symbol):
			builder.WriteRune(symbol)
			previousSpace = false
		case unicode.IsSpace(symbol):
			if !previousSpace && builder.Len() > 0 {
				builder.WriteRune(' ')
				previousSpace = true
			}
		}
	}
	return strings.TrimSpace(builder.String())
}
