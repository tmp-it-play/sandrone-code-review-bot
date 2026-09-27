package progresscomment

import "strings"

const checkTitleMaxRunes = 200

func CheckTitle(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), ">"))
		if line == "" || strings.HasPrefix(line, "<!--") || strings.HasPrefix(line, "[!") && strings.HasSuffix(line, "]") {
			continue
		}
		runes := []rune(line)
		if len(runes) > checkTitleMaxRunes {
			return string(runes[:checkTitleMaxRunes-1]) + "…"
		}
		return line
	}
	return ""
}
