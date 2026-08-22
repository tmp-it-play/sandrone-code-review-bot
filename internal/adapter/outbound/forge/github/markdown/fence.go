package markdown

import "strings"

func fenceFor(content string) string {
	longest := 0
	current := 0
	for _, symbol := range content {
		if symbol == '`' {
			current++
			if current > longest {
				longest = current
			}
			continue
		}
		current = 0
	}
	if longest < 3 {
		return "```"
	}
	return strings.Repeat("`", longest+1)
}

func normalizeSuggestion(content string) string {
	trimmed := strings.TrimLeft(content, "\r\n")
	return strings.TrimRight(trimmed, " \t\r\n")
}
