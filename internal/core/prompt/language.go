package prompt

import "github.com/it-play/sandrone-code-review-bot/internal/core/setting"

func languageName(code string) string {
	switch setting.LanguageCode(code) {
	case "ko":
		return "한국어"
	case "en":
		return "영어"
	case "ja":
		return "일본어"
	default:
		return code
	}
}
