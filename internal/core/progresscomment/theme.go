package progresscomment

import "strings"

type Theme string

const (
	ThemeProgramming Theme = "programming"
	ThemeSandrone    Theme = "sandrone"
	ThemeStock       Theme = "stock"
	ThemeHistory     Theme = "history"
)

func ParseTheme(value string) (Theme, bool) {
	switch Theme(strings.ToLower(strings.TrimSpace(value))) {
	case ThemeProgramming:
		return ThemeProgramming, true
	case ThemeSandrone:
		return ThemeSandrone, true
	case ThemeStock:
		return ThemeStock, true
	case ThemeHistory:
		return ThemeHistory, true
	default:
		return "", false
	}
}

func NormalizeTheme(theme Theme) Theme {
	parsed, valid := ParseTheme(string(theme))
	if !valid {
		return ThemeProgramming
	}
	return parsed
}
