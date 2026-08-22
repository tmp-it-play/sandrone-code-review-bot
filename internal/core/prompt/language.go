package prompt

import "strings"

func languageName(code string) string {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "", "ko", "ko-kr", "korean":
		return "한국어"
	case "en", "en-us", "english":
		return "영어"
	case "ja", "jp", "ja-jp", "jp-jp", "japanese":
		return "일본어"
	default:
		return code
	}
}
