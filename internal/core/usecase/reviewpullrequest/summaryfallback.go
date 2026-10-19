package reviewpullrequest

import "github.com/it-play/sandrone-code-review-bot/internal/core/setting"

func summaryFallback(config setting.RepoConfig) string {
	switch setting.LanguageCode(config.Language) {
	case "ko":
		switch config.Tone {
		case setting.ToneSandrone:
			return "변경 내용 요약을 만들지 못했어. 검토 결과는 아래에서 확인하면 돼."
		case setting.TonePolite:
			return "변경 내용 요약을 제공하지 못했어요. 검토 결과는 아래에서 확인해 주세요."
		default:
			return "변경 내용 요약을 제공하지 못했습니다. 검토 결과는 아래에 표시합니다."
		}
	case "ja":
		if config.Tone == setting.ToneSandrone {
			return "変更内容の概要を作成できなかったわ。レビュー結果は以下で確認して。"
		}
		return "変更内容の概要を作成できませんでした。レビュー結果を以下に示します。"
	default:
		if config.Tone == setting.ToneSandrone {
			return "The change summary isn’t available. Check the review results below."
		}
		return "A change summary is unavailable. Review results are shown below."
	}
}
