package markdown

import (
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"github.com/it-play/sandrone-code-review-bot/internal/core/setting"
)

type reviewProse struct {
	summaryHeading  string
	noFindings      string
	omittedFindings string
	fileCount       string
	fileHeader      string
	changesHeader   string
	fallbackCount   string
	suggestion      string
	otherLocations  string
	moreLocations   string
	incremental     string
	input           string
	output          string
	total           string
	tokens          string
	multipleModels  string
	replyHint       string
	repositoryLink  string
	severities      map[review.Severity]string
}

func reviewProseFor(style review.Style) reviewProse {
	switch setting.LanguageCode(style.Language) {
	case "ko":
		return koreanReviewProse(setting.Tone(style.Tone))
	case "ja":
		return japaneseReviewProse(setting.Tone(style.Tone))
	default:
		return englishReviewProse(setting.Tone(style.Tone))
	}
}
