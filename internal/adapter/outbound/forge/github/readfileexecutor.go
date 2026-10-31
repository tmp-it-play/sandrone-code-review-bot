package github

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
	"github.com/it-play/sandrone-code-review-bot/internal/core/port/outbound"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
)

type ReadFileExecutor struct {
	content   outbound.RepositoryContent
	masker    outbound.Masker
	target    pullrequest.Target
	ref       string
	maxChars  int
	mutex     sync.Mutex
	remaining int
}

func (e *ReadFileExecutor) Definitions() []llm.Tool {
	return []llm.Tool{{
		Name:        "read_file",
		Description: "저장소에서 파일 하나의 현재 내용을 읽는다. 리뷰 판단에 주변 코드가 필요할 때만 사용하며 finding 위치와 evidence는 반드시 changed_files의 추가 diff 줄에서 고른다.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"end_line":   map[string]any{"type": "integer", "minimum": 1, "description": "마지막 줄 번호(포함). 생략하면 출력 한도까지 읽는다."},
				"start_line": map[string]any{"type": "integer", "minimum": 1, "description": "읽기 시작할 1부터의 줄 번호. 기본은 첫 줄."},
				"path": map[string]any{
					"type":        "string",
					"description": "저장소 루트 기준 파일 경로",
				},
			},
			"required": []string{"path"},
		},
	}}
}

func (e *ReadFileExecutor) Execute(ctx context.Context, call llm.ToolCall) (string, error) {
	if call.Name != "read_file" {
		return "", fmt.Errorf("알 수 없는 도구다: %s", call.Name)
	}
	e.mutex.Lock()
	if e.remaining <= 0 {
		e.mutex.Unlock()
		return "더 이상 파일을 읽을 수 없다. 지금까지 읽은 내용으로 판단한다.", nil
	}
	e.remaining--
	e.mutex.Unlock()

	var arguments struct {
		Path      string `json:"path"`
		StartLine *int   `json:"start_line"`
		EndLine   *int   `json:"end_line"`
	}
	if err := json.Unmarshal([]byte(call.Arguments), &arguments); err != nil {
		return "인자를 해석하지 못했다. path를 문자열로 넘긴다.", nil
	}
	if arguments.Path == "" {
		return "path가 비어 있다.", nil
	}
	start := 1
	if arguments.StartLine != nil {
		start = *arguments.StartLine
	}
	if start < 1 || arguments.EndLine != nil && *arguments.EndLine < start {
		return "줄 범위가 올바르지 않다.", nil
	}
	body, err := e.content.File(ctx, e.target, arguments.Path, e.ref)
	if err != nil {
		return fmt.Sprintf("%s 파일을 읽지 못했다.", arguments.Path), nil
	}
	body = e.masker.Mask(body)
	if start > 1 || arguments.EndLine != nil {
		for line := 1; line < start; line++ {
			_, tail, found := strings.Cut(body, "\n")
			if !found || tail == "" {
				return "요청한 시작 줄이 파일 끝을 넘었다.", nil
			}
			body = tail
		}
		if arguments.EndLine != nil {
			end := 0
			for line := start; ; line++ {
				newline := strings.IndexByte(body[end:], '\n')
				if newline < 0 {
					break
				}
				end += newline + 1
				if line == *arguments.EndLine {
					body = body[:end]
					break
				}
			}
		}
		body = fmt.Sprintf("[파일 일부: 원문 %d번째 줄부터]\n%s", start, body)
	}
	if e.maxChars > 0 && len(body) > e.maxChars {
		limit := e.maxChars
		for limit > 0 && !utf8.RuneStart(body[limit]) {
			limit--
		}
		body = body[:limit] + "\n[분량 제한으로 이후 내용 생략]"
	}
	return body, nil
}
