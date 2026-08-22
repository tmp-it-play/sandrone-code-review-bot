package prompt

import (
	"fmt"
	"strings"

	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
)

type ReviewPrompt struct {
	Context      Context
	Extra        string
	ToolsAllowed bool
}

func (p ReviewPrompt) Messages() []llm.Message {
	return []llm.Message{
		{Role: llm.RoleSystem, Content: p.system()},
		{Role: llm.RoleUser, Content: p.user()},
	}
}

func (p ReviewPrompt) system() string {
	var builder strings.Builder
	builder.WriteString("당신은 GitHub Pull Request를 검토하는 코드 리뷰어다.\n\n")
	builder.WriteString("원칙:\n")
	builder.WriteString("- 변경된 코드에서 문제가 되는 부분과 개선 여지가 뚜렷한 부분을 함께 짚는다.\n")
	builder.WriteString("- 지적마다 어떤 입력·상황에서 어떻게 잘못되는지 근거를 적는다. 근거를 댈 수 없으면 적지 않는다.\n")
	builder.WriteString("- 버그·보안·성능처럼 동작이 틀어지는 문제는 major 이상으로 남긴다.\n")
	builder.WriteString("- 가독성·중복·네이밍·예외 처리처럼 고치면 나아지는 부분은 minor나 nit으로 남긴다.\n")
	builder.WriteString("- 단순 취향 차이나 이미 프로젝트 규칙을 따르는 코드는 지적하지 않는다.\n")
	builder.WriteString("- 저장소 규칙 문서가 주어지면 그 규칙을 우선한다.\n")
	builder.WriteString("- diff에 없는 줄은 지적하지 않는다.\n")
	builder.WriteString(fmt.Sprintf("- 모든 서술은 %s로 작성한다.\n\n", languageName(p.Context.Config.Language)))
	builder.WriteString(toneGuide(p.Context.Config.Tone))
	if p.ToolsAllowed {
		builder.WriteString("판단에 주변 코드가 더 필요하면 read_file 도구로 파일을 읽을 수 있다.\n\n")
	}
	builder.WriteString("심각도:\n")
	builder.WriteString("- critical: 데이터 손실·보안 취약점·운영 장애로 이어짐\n")
	builder.WriteString("- major: 특정 조건에서 잘못 동작하거나 명백한 버그\n")
	builder.WriteString("- minor: 동작은 하지만 개선이 필요함\n")
	builder.WriteString("- nit: 사소한 정리 제안\n\n")
	builder.WriteString(reviewSchema)
	return builder.String()
}

func (p ReviewPrompt) user() string {
	var builder strings.Builder
	builder.WriteString(p.Context.Render())
	if extra := strings.TrimSpace(p.Extra); extra != "" {
		builder.WriteString("\n<extra_instruction>\n")
		builder.WriteString(extra)
		builder.WriteString("\n</extra_instruction>\n")
	}
	builder.WriteString("\n위 변경을 검토하고 지정된 JSON 형식으로만 답한다.\n")
	return builder.String()
}
