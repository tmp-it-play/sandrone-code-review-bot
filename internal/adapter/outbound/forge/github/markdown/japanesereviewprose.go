package markdown

import (
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"github.com/it-play/sandrone-code-review-bot/internal/core/setting"
)

func japaneseReviewProse(tone setting.Tone) reviewProse {
	prose := reviewProse{
		summaryHeading: "Pull Request の概要", noFindings: "確認した範囲では、指摘すべき問題はありません。",
		omittedFindings: "出力サイズの上限により、優先度の低い指摘を%d件省略しました。",
		fileCount:       "ファイル別の変更内容（%d件）", fileHeader: "ファイル", changesHeader: "変更内容",
		fallbackCount: "インラインに配置できなかった指摘（%d件）", suggestion: "修正案:",
		otherLocations: "同じ原因がある他の箇所: ", moreLocations: "、ほか%d箇所", incremental: "前回のレビュー以降の変更",
		input: "入力", output: "出力", total: "合計", tokens: "トークン", multipleModels: "複数モデル",
		replyHint:      "インラインのレビューコメントで `/pr-review-reply` を入力すると、指摘が解消されたかを再確認します。",
		repositoryLink: "Sandrone Code Review Bot リポジトリ",
		severities:     map[review.Severity]string{review.SeverityCritical: "致命的", review.SeverityMajor: "重大", review.SeverityMinor: "軽微", review.SeverityNit: "些細"},
	}
	switch tone {
	case setting.ToneIntelligent:
		prose.noFindings = "確認した範囲では、根拠のある問題は検出されませんでした。"
	case setting.TonePolite:
		prose.noFindings = "確認した範囲では、お伝えすべき問題は見つかりませんでした。"
		prose.replyHint = "インラインのレビューコメントに `/pr-review-reply` と入力していただければ、指摘が解消されたかを再確認します。"
	case setting.ToneSandrone:
		prose.noFindings = "確認した範囲では、指摘するような欠陥はないわ。"
		prose.omittedFindings = "出力サイズの上限で、優先度の低い指摘を%d件省略したわ。"
		prose.replyHint = "インラインのコメントに `/pr-review-reply` と入力すれば、欠陥が解消されたか確認するわ。"
	}
	return prose
}
