package masking

import "regexp"

type SecretMasker struct {
	literals []*regexp.Regexp
	keyed    []*regexp.Regexp
}

func NewSecretMasker() SecretMasker {
	return SecretMasker{
		literals: []*regexp.Regexp{
			regexp.MustCompile(`(?i)-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----`),
			regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{16,}`),
			regexp.MustCompile(`github_pat_[A-Za-z0-9_]{20,}`),
			regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
			regexp.MustCompile(`AIza[0-9A-Za-z\-_]{30,}`),
			regexp.MustCompile(`sk-[A-Za-z0-9\-_]{20,}`),
			regexp.MustCompile(`xox[abprs]-[A-Za-z0-9\-]{10,}`),
			regexp.MustCompile(`nvapi-[A-Za-z0-9\-_]{20,}`),
			regexp.MustCompile(`gsk_[A-Za-z0-9]{20,}`),
			regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9\-._~+/]{20,}`),
			regexp.MustCompile(`eyJ[A-Za-z0-9\-_]{10,}\.[A-Za-z0-9\-_]{10,}\.[A-Za-z0-9\-_]{10,}`),
		},
		keyed: []*regexp.Regexp{
			regexp.MustCompile(`(?i)((?:api[_-]?key|secret|password|passwd|token|credential)[a-z_]*\s*[:=]\s*)(["']?)[A-Za-z0-9\-._~+/]{12,}(["']?)`),
		},
	}
}

func (m SecretMasker) Mask(text string) string {
	if text == "" {
		return text
	}
	masked := text
	for _, pattern := range m.literals {
		masked = pattern.ReplaceAllString(masked, "[REDACTED]")
	}
	for _, pattern := range m.keyed {
		masked = pattern.ReplaceAllString(masked, "${1}${2}[REDACTED]${3}")
	}
	return masked
}
