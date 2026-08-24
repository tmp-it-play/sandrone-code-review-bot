package prompt

import (
	"fmt"
	"strings"
	"unicode/utf8"

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
	Inventory    []pullrequest.ChangedFile
}

func (c Context) Render() string {
	required := c.renderPullRequest() + c.renderFiles()
	optional := c.renderInstructions() + c.renderInventory()
	if c.Config.MaxPromptChars <= 0 || len(required)+len(optional) <= c.Config.MaxPromptChars {
		return required + optional
	}
	remaining := c.Config.MaxPromptChars - len(required)
	if remaining <= 0 {
		return required
	}
	notice := "\n\n[부가 컨텍스트가 길어 이후 내용은 생략되었다]\n"
	keep := remaining - len(notice)
	if keep <= 0 {
		return required
	}
	for keep > 0 && !utf8.RuneStart(optional[keep]) {
		keep--
	}
	return required + optional[:keep] + notice
}

func (c Context) renderPullRequest() string {
	var builder strings.Builder
	builder.WriteString("<pull_request>\n")
	fmt.Fprintf(&builder, "제목: %s\n", c.PullRequest.Title)
	fmt.Fprintf(&builder, "작성자: %s\n", c.PullRequest.Author)
	fmt.Fprintf(&builder, "대상 브랜치: %s\n", c.PullRequest.BaseRef)
	if body, truncated := truncateBody(c.PullRequest.Body); body != "" {
		builder.WriteString("본문:\n")
		builder.WriteString(body)
		if truncated {
			builder.WriteString("\n[본문이 길어 이후 내용은 생략되었다]")
		}
		builder.WriteString("\n")
	}
	if c.Incremental {
		builder.WriteString("범위: 직전 리뷰 이후 추가된 커밋의 변경분만\n")
	}
	builder.WriteString("</pull_request>\n\n")
	return builder.String()
}

func (c Context) renderInventory() string {
	if len(c.Inventory) == 0 {
		return ""
	}
	var builder strings.Builder
	builder.WriteString("<all_changed_files>\n")
	builder.WriteString("이 PR이 건드린 파일 전체다. 이 중 아래 <changed_files>에 실린 파일만 이번 요청에서 검토한다.\n")
	for _, file := range c.Inventory {
		fmt.Fprintf(&builder, "- %s (+%d/-%d)\n", file.Path, file.Additions, file.Deletions)
	}
	builder.WriteString("</all_changed_files>\n\n")
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
		fmt.Fprintf(&builder, "--- %s ---\n", document.Path)
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
		fmt.Fprintf(&builder, "<file path=\"%s\" status=\"%s\" additions=\"%d\" deletions=\"%d\">\n", file.Path, file.Status, file.Additions, file.Deletions)
		if strings.TrimSpace(file.Patch) != "" {
			builder.WriteString("<diff>\n")
			builder.WriteString(strings.TrimRight(file.Patch, "\r\n"))
			builder.WriteString("\n</diff>\n")
		}
		if strings.TrimSpace(file.Content) != "" {
			builder.WriteString("<current_content>\n")
			builder.WriteString(strings.TrimRight(file.Content, "\r\n"))
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
