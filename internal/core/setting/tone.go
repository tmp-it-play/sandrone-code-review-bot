package setting

import "strings"

type Tone string

const (
	ToneProfessional Tone = "professional"
	ToneIntelligent  Tone = "intelligent"
	TonePolite       Tone = "polite"
	ToneSandrone     Tone = "sandrone"
)

func ParseTone(value string) (Tone, bool) {
	switch Tone(strings.ToLower(strings.TrimSpace(value))) {
	case ToneProfessional:
		return ToneProfessional, true
	case ToneIntelligent:
		return ToneIntelligent, true
	case TonePolite:
		return TonePolite, true
	case ToneSandrone:
		return ToneSandrone, true
	default:
		return "", false
	}
}
