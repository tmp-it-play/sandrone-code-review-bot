package prompt

import (
	"fmt"
	"strings"

	"github.com/it-play/sandrone-code-review-bot/internal/core/instruction"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/setting"
)

type Context struct {
	PullRequest  pullrequest.PullRequest
	Files        []pullrequest.ChangedFile
	Instructions instruction.Collection
	Config       setting.RepoConfig
	Incremental  bool
}

func (c Context) Render() string {
	var builder strings.Builder
	builder.WriteString(c.renderPullRequest())
	builder.WriteString(c.renderInstructions())
	builder.WriteString(c.renderFiles())
	rendered := builder.String()
	if c.Config.MaxPromptChars > 0 && len(rendered) > c.Config.MaxPromptChars {
		return rendered[:c.Config.MaxPromptChars] + "\n\n[컨텍스트가 길어 이후 내용은 생략되었다]\n"
	}
	return rendered
}

func (c Context) renderPullRequest() string {
	var builder strings.Builder
	builder.WriteString("<pull_request>\n")
	builder.WriteString(fmt.Sprintf("제목: %s\n", c.PullRequest.Title))
	builder.WriteString(fmt.Sprintf("작성자: %s\n", c.PullRequest.Author))
	builder.WriteString(fmt.Sprintf("대상 브랜치: %s\n", c.PullRequest.BaseRef))
	if body := strings.TrimSpace(c.PullRequest.Body); body != "" {
		builder.WriteString("본문:\n")
		builder.WriteString(body)
		builder.WriteString("\n")
	}
	if c.Incremental {
		builder.WriteString("범위: 직전 리뷰 이후 추가된 커밋의 변경분만\n")
	}
	builder.WriteString("</pull_request>\n\n")
	return builder.String()
}

func (c Context) renderInstructions() string {
	if c.Instructions.IsEmpty() && len(c.Instructions.Omitted) == 0 {
		return ""
	}
	var builder strings.Builder
	builder.WriteString("<project_rules>\n")
	builder.WriteString("이 저장소의 규칙 문서다. 리뷰 기준으로 삼는다.\n\n")
	for _, document := range c.Instructions.Documents {
		builder.WriteString(fmt.Sprintf("--- %s ---\n", document.Path))
		builder.WriteString(document.Content)
		if document.Truncated {
			builder.WriteString("\n[분량 제한으로 이후 내용 생략]")
		}
		builder.WriteString("\n\n")
	}
	if len(c.Instructions.Omitted) > 0 {
		builder.WriteString("분량 제한으로 본문을 싣지 못한 문서: ")
		builder.WriteString(strings.Join(c.Instructions.Omitted, ", "))
		builder.WriteString("\n")
	}
	builder.WriteString("</project_rules>\n\n")
	return builder.String()
}

func (c Context) renderFiles() string {
	var builder strings.Builder
	builder.WriteString("<changed_files>\n")
	for _, file := range c.Files {
		builder.WriteString(fmt.Sprintf("<file path=\"%s\" status=\"%s\" additions=\"%d\" deletions=\"%d\">\n", file.Path, file.Status, file.Additions, file.Deletions))
		if patch := strings.TrimSpace(file.Patch); patch != "" {
			builder.WriteString("<diff>\n")
			builder.WriteString(patch)
			builder.WriteString("\n</diff>\n")
		}
		if content := strings.TrimSpace(file.Content); content != "" {
			builder.WriteString("<current_content>\n")
			builder.WriteString(content)
			if file.Truncated {
				builder.WriteString("\n[분량 제한으로 이후 내용 생략]")
			}
			builder.WriteString("\n</current_content>\n")
		}
		builder.WriteString("</file>\n")
	}
	builder.WriteString("</changed_files>\n")
	return builder.String()
}
