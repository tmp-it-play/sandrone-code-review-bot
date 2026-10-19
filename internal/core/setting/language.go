package setting

import "strings"

func LanguageCode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "ko", "ko-kr", "korean":
		return "ko"
	case "en", "en-us", "english":
		return "en"
	case "ja", "jp", "ja-jp", "jp-jp", "japanese":
		return "ja"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}
