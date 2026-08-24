package masking

import (
	"regexp"
	"strings"
)

type SecretMasker struct {
	literals      []*regexp.Regexp
	quotedKeyed   []*regexp.Regexp
	unquotedKeyed []*regexp.Regexp
	credentialURI []*regexp.Regexp
	lineKeyed     *regexp.Regexp
	blockScalar   *regexp.Regexp
}

func NewSecretMasker() SecretMasker {
	return SecretMasker{
		literals: []*regexp.Regexp{
			regexp.MustCompile(`(?i)-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----`),
			regexp.MustCompile(`(?i)-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*\z`),
			regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{16,}`),
			regexp.MustCompile(`github_pat_[A-Za-z0-9_]{20,}`),
			regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
			regexp.MustCompile(`AIza[0-9A-Za-z\-_]{30,}`),
			regexp.MustCompile(`sk-[A-Za-z0-9\-_]{20,}`),
			regexp.MustCompile(`xox[abprs]-[A-Za-z0-9\-]{10,}`),
			regexp.MustCompile(`nvapi-[A-Za-z0-9\-_]{20,}`),
			regexp.MustCompile(`gsk_[A-Za-z0-9]{20,}`),
			regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9\-._~+/]{20,}`),
			regexp.MustCompile(`(?i)authorization\s*:\s*basic\s+[A-Za-z0-9+/=]{8,}`),
			regexp.MustCompile(`eyJ[A-Za-z0-9\-_]{10,}\.[A-Za-z0-9\-_]{10,}\.[A-Za-z0-9\-_]{10,}`),
		},
		quotedKeyed: []*regexp.Regexp{
			regexp.MustCompile(`(?i)(["']?(?:api[_-]?key|secret|password|passwd|token|credential|cookie|set[_-]?cookie|dsn|database[_-]?url|redis[_-]?url|connection[_-]?string|jdbc[_-]?url)[a-z_]*["']?\s*[:=]\s*)(["'])[^"'\r\n]{4,}`),
		},
		unquotedKeyed: []*regexp.Regexp{
			regexp.MustCompile(`(?i)(["']?(?:api[_-]?key|secret|password|passwd|token|credential|cookie|set[_-]?cookie|dsn|database[_-]?url|redis[_-]?url|connection[_-]?string|jdbc[_-]?url)[a-z_]*["']?\s*[:=]\s*)[^\s,}\]\r\n#]{4,}`),
		},
		credentialURI: []*regexp.Regexp{
			regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://[^/\s:@]*:)[^/\s]+(@[^/\s]+)`),
		},
		lineKeyed:   regexp.MustCompile(`(?i)^([+\-]?[\t ]*["']?[A-Za-z0-9_-]*(?:api[_-]?key|secret|password|passwd|token|credential|cookie|set[_-]?cookie|dsn|database[_-]?url|redis[_-]?url|connection[_-]?string|jdbc[_-]?url)[A-Za-z0-9_-]*["']?[\t ]*[:=][\t ]*)(.*)$`),
		blockScalar: regexp.MustCompile(`^[|>][-+0-9]*([\t ]*#.*)?$`),
	}
}

func (m SecretMasker) Mask(text string) string {
	if text == "" {
		return text
	}
	masked := text
	for _, pattern := range m.literals {
		masked = pattern.ReplaceAllStringFunc(masked, redactLiteral)
	}
	for _, pattern := range m.quotedKeyed {
		masked = pattern.ReplaceAllString(masked, "${1}${2}[REDACTED]")
	}
	for _, pattern := range m.unquotedKeyed {
		masked = pattern.ReplaceAllString(masked, "${1}[REDACTED]")
	}
	for _, pattern := range m.credentialURI {
		masked = pattern.ReplaceAllString(masked, "${1}[REDACTED]${2}")
	}
	return m.maskKeyedLines(masked)
}

func (m SecretMasker) maskKeyedLines(text string) string {
	var masked strings.Builder
	remaining := text
	blockIndent := -1
	for remaining != "" {
		line, ending, rest := splitMaskLine(remaining)
		remaining = rest
		indent := maskIndent(line)
		if blockIndent >= 0 {
			if strings.TrimSpace(strings.TrimLeft(line, "+-")) == "" || indent > blockIndent {
				masked.WriteString(maskLineContent(line))
				masked.WriteString(ending)
				continue
			}
			blockIndent = -1
		}
		indices := m.lineKeyed.FindStringSubmatchIndex(line)
		if indices == nil {
			masked.WriteString(line)
			masked.WriteString(ending)
			continue
		}
		prefixEnd := indices[3]
		value := line[indices[4]:indices[5]]
		if strings.HasPrefix(strings.TrimSpace(value), "=") && strings.HasSuffix(strings.TrimSpace(line[:prefixEnd]), ":") {
			masked.WriteString(line)
			masked.WriteString(ending)
			continue
		}
		masked.WriteString(line[:prefixEnd])
		masked.WriteString("[REDACTED]")
		masked.WriteString(ending)
		if m.blockScalar.MatchString(strings.TrimSpace(value)) {
			blockIndent = indent
		}
	}
	return masked.String()
}

func splitMaskLine(value string) (string, string, string) {
	newline := strings.IndexAny(value, "\r\n")
	if newline < 0 {
		return value, "", ""
	}
	width := 1
	if value[newline] == '\r' && newline+1 < len(value) && value[newline+1] == '\n' {
		width = 2
	}
	return value[:newline], value[newline : newline+width], value[newline+width:]
}

func maskIndent(line string) int {
	index := 0
	if len(line) > 0 && (line[0] == '+' || line[0] == '-') {
		index++
	}
	indent := 0
	for index < len(line) && (line[index] == ' ' || line[index] == '\t') {
		indent++
		index++
	}
	return indent
}

func maskLineContent(line string) string {
	index := 0
	if len(line) > 0 && (line[0] == '+' || line[0] == '-') {
		index++
	}
	for index < len(line) && (line[index] == ' ' || line[index] == '\t') {
		index++
	}
	return line[:index] + "[REDACTED]"
}

func redactLiteral(value string) string {
	if !strings.ContainsAny(value, "\r\n") {
		return "[REDACTED]"
	}
	var redacted strings.Builder
	remaining := value
	firstLine := true
	for remaining != "" {
		line := remaining
		lineEnding := ""
		if newlineIndex := strings.IndexAny(remaining, "\r\n"); newlineIndex >= 0 {
			line = remaining[:newlineIndex]
			endingWidth := 1
			if remaining[newlineIndex] == '\r' && newlineIndex+1 < len(remaining) && remaining[newlineIndex+1] == '\n' {
				endingWidth = 2
			}
			lineEnding = remaining[newlineIndex : newlineIndex+endingWidth]
			remaining = remaining[newlineIndex+endingWidth:]
		} else {
			remaining = ""
		}
		if !firstLine && (strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") || strings.HasPrefix(line, " ")) {
			redacted.WriteByte(line[0])
		}
		redacted.WriteString("[REDACTED]")
		redacted.WriteString(lineEnding)
		firstLine = false
	}
	return redacted.String()
}
