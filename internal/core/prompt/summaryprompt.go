package prompt

import (
	"fmt"
	"strings"

	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
)

type SummaryPrompt struct {
	Context Context
	Extra   string
}

func (p SummaryPrompt) Messages() []llm.Message {
	return []llm.Message{
		{Role: llm.RoleSystem, Content: p.system()},
		{Role: llm.RoleUser, Content: p.user()},
	}
}

func (p SummaryPrompt) system() string {
	var builder strings.Builder
	builder.WriteString("당신은 GitHub Pull Request의 변경 내용을 요약하는 리뷰어다.\n\n")
	builder.WriteString("원칙:\n")
	builder.WriteString("- 무엇이 왜 바뀌었는지를 읽는 사람이 코드를 열지 않고도 알 수 있게 쓴다.\n")
	builder.WriteString("- diff에서 확인되지 않는 의도를 지어내지 않는다.\n")
	builder.WriteString("- 문제 지적은 하지 않는다. 요약만 한다.\n")
	builder.WriteString(fmt.Sprintf("- 모든 서술은 %s로 작성한다.\n\n", languageName(p.Context.Config.Language)))
	builder.WriteString(toneGuide)
	builder.WriteString(summarySchema)
	return builder.String()
}

func (p SummaryPrompt) user() string {
	var builder strings.Builder
	builder.WriteString(p.Context.Render())
	if extra := strings.TrimSpace(p.Extra); extra != "" {
		builder.WriteString("\n<extra_instruction>\n")
		builder.WriteString(extra)
		builder.WriteString("\n</extra_instruction>\n")
	}
	builder.WriteString("\n위 변경을 요약하고 지정된 JSON 형식으로만 답한다.\n")
	return builder.String()
}
