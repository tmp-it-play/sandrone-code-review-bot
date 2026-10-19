package textrevision

import (
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
)

type Checkpoint struct {
	InputHash string
	Edits     []Edit
	Fallback  bool
	Response  llm.Response
}

func (c Checkpoint) Encode() (string, error) {
	if err := c.Validate(); err != nil {
		return "", err
	}
	encoded, err := json.Marshal(c)
	return string(encoded), err
}

func DecodeCheckpoint(encoded string) (Checkpoint, error) {
	var checkpoint Checkpoint
	if err := json.Unmarshal([]byte(encoded), &checkpoint); err != nil {
		return checkpoint, err
	}
	return checkpoint, checkpoint.Validate()
}

func (c Checkpoint) Validate() error {
	if _, err := hex.DecodeString(c.InputHash); err != nil || len(c.InputHash) != 64 {
		return errors.New("문체 보정 입력 식별자가 유효하지 않습니다")
	}
	if c.Response.Content != "" || len(c.Response.ToolCalls) != 0 || c.Response.ToolExecutions < 0 || c.Response.Usage.PromptTokens < 0 || c.Response.Usage.CompletionTokens < 0 || c.Response.Usage.TotalTokens < 0 {
		return errors.New("문체 보정 사용량 정보가 유효하지 않습니다")
	}
	return nil
}
