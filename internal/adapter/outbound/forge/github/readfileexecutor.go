package github

import (
	"context"
	"encoding/json"
	"fmt"
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
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(call.Arguments), &arguments); err != nil {
		return "인자를 해석하지 못했다. path를 문자열로 넘긴다.", nil
	}
	if arguments.Path == "" {
		return "path가 비어 있다.", nil
	}
	body, err := e.content.File(ctx, e.target, arguments.Path, e.ref)
	if err != nil {
		return fmt.Sprintf("%s 파일을 읽지 못했다.", arguments.Path), nil
	}
	body = e.masker.Mask(body)
	if e.maxChars > 0 && len(body) > e.maxChars {
		limit := e.maxChars
		for limit > 0 && !utf8.RuneStart(body[limit]) {
			limit--
		}
		body = body[:limit] + "\n[분량 제한으로 이후 내용 생략]"
	}
	return body, nil
}
