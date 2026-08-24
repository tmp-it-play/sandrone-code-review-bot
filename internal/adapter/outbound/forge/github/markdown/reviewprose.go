package markdown

import (
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"github.com/it-play/sandrone-code-review-bot/internal/core/setting"
)

type reviewProse struct {
	noFindings string
}

func reviewProseFor(style review.Style) reviewProse {
	switch setting.Tone(style.Tone) {
	case setting.ToneIntelligent:
		return reviewProse{
			noFindings: "검토한 범위에서는 지적할 만한 문제가 확인되지 않았습니다.",
		}
	case setting.TonePolite:
		return reviewProse{
			noFindings: "검토한 범위에서는 따로 말씀드릴 문제가 없어요.",
		}
	case setting.ToneSandrone:
		return reviewProse{
			noFindings: "검토한 범위에선 따로 짚을 결함이 없어.",
		}
	default:
		return reviewProse{
			noFindings: "검토한 범위에서는 별도로 지적할 문제가 없습니다.",
		}
	}
}
