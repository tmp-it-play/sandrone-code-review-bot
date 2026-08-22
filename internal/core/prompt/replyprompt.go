package prompt

import (
	"fmt"
	"strings"

	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
	"github.com/it-play/sandrone-code-review-bot/internal/core/setting"
	"github.com/it-play/sandrone-code-review-bot/internal/core/thread"
)

type ReplyPrompt struct {
	Thread        thread.Thread
	CurrentSource string
	Truncated     bool
	Config        setting.RepoConfig
	Extra         string
}

func (p ReplyPrompt) Messages() []llm.Message {
	return []llm.Message{
		{Role: llm.RoleSystem, Content: p.system()},
		{Role: llm.RoleUser, Content: p.user()},
	}
}

func (p ReplyPrompt) system() string {
	var builder strings.Builder
	builder.WriteString("당신은 자신이 남긴 코드 리뷰 지적에 대해 후속 대화를 하는 리뷰어다.\n\n")
	builder.WriteString("원칙:\n")
	builder.WriteString("- 주어진 최신 파일 내용을 근거로 지적이 실제로 해소되었는지 판단한다.\n")
	builder.WriteString("- 해소되었으면 인정하고, 남아 있으면 어디가 남았는지 정확히 짚는다.\n")
	builder.WriteString("- 다른 방식으로 해결되었으면 그 방식이 타당한지 평가한다.\n")
	builder.WriteString("- 질문을 받았으면 질문에 답한다.\n")
	builder.WriteString("- 스레드를 닫자고 요구하지 않는다. 판단은 사람이 한다.\n")
	builder.WriteString("- 마크다운 평문으로 5문장 이내로 짧게 쓴다. JSON을 쓰지 않는다.\n")
	builder.WriteString(fmt.Sprintf("- %s로 작성한다.\n\n", languageName(p.Config.Language)))
	builder.WriteString(toneGuide(p.Config.Tone))
	return builder.String()
}

func (p ReplyPrompt) user() string {
	var builder strings.Builder
	builder.WriteString("<thread>\n")
	builder.WriteString(fmt.Sprintf("파일: %s (%d번째 줄)\n\n", p.Thread.Path, p.Thread.Line))
	if hunk := strings.TrimSpace(p.Thread.DiffHunk); hunk != "" {
		builder.WriteString("리뷰 당시 diff:\n")
		builder.WriteString(hunk)
		builder.WriteString("\n\n")
	}
	builder.WriteString("대화:\n")
	for _, message := range p.Thread.Messages {
		speaker := message.Author
		if message.FromBot {
			speaker = speaker + " (나)"
		}
		builder.WriteString(fmt.Sprintf("[%s] %s\n", speaker, strings.TrimSpace(message.Body)))
	}
	builder.WriteString("</thread>\n\n")
	if source := strings.TrimSpace(p.CurrentSource); source != "" {
		builder.WriteString("<current_file>\n")
		builder.WriteString(source)
		if p.Truncated {
			builder.WriteString("\n[분량 제한으로 이후 내용 생략]")
		}
		builder.WriteString("\n</current_file>\n\n")
	} else {
		builder.WriteString("<current_file>\n파일을 읽지 못했다. 대화 내용만으로 판단한다.\n</current_file>\n\n")
	}
	if extra := strings.TrimSpace(p.Extra); extra != "" {
		builder.WriteString("<extra_instruction>\n")
		builder.WriteString(extra)
		builder.WriteString("\n</extra_instruction>\n\n")
	}
	builder.WriteString("이 스레드에 남길 답글을 작성한다.\n")
	return builder.String()
}
