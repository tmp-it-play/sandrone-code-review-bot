package markdown

import (
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"github.com/it-play/sandrone-code-review-bot/internal/core/setting"
)

type reviewProse struct {
	noFindings      string
	unreviewedFiles string
}

func reviewProseFor(style review.Style) reviewProse {
	switch setting.Tone(style.Tone) {
	case setting.ToneIntelligent:
		return reviewProse{
			noFindings:      "검토한 범위에서는 지적할 만한 문제가 확인되지 않았습니다.",
			unreviewedFiles: "아래 파일은 입력 분량 한도를 초과해 이번 검토에서 제외되었습니다. 전체 검토가 필요하면 범위를 나누어 다시 요청해야 합니다.",
		}
	case setting.TonePolite:
		return reviewProse{
			noFindings:      "검토한 범위에서는 따로 말씀드릴 문제가 없어요.",
			unreviewedFiles: "아래 파일은 분량 제한으로 이번 리뷰에 담지 못했어요. 확인이 필요하시면 범위를 좁혀 다시 요청해 주세요.",
		}
	case setting.ToneSandrone:
		return reviewProse{
			noFindings:      "검토한 범위에선 따로 짚을 결함이 없어.",
			unreviewedFiles: "아래 파일은 분량 한도를 넘어 이번 검토 대상에서 제외했어. 전부 확인하려면 범위를 나눠 다시 요청하면 돼.",
		}
	default:
		return reviewProse{
			noFindings:      "검토한 범위에서는 별도로 지적할 문제가 없습니다.",
			unreviewedFiles: "아래 파일은 분량 제한으로 이번 리뷰에 포함하지 못했습니다. 확인이 필요하면 범위를 좁혀 다시 요청해 주세요.",
		}
	}
}
