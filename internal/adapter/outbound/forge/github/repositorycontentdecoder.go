package github

import (
	"errors"
	"fmt"

	gh "github.com/google/go-github/v90/github"
	"github.com/it-play/sandrone-code-review-bot/internal/core/port/outbound"
)

func decodeRepositoryContent(path string, file *gh.RepositoryContent) (string, error) {
	if file == nil {
		return "", fmt.Errorf("%s는 파일이 아닙니다: %w", path, outbound.ErrRepositoryContentUnavailable)
	}
	content, err := file.GetContent()
	if err != nil {
		return "", fmt.Errorf("%s 내용을 해석하지 못했습니다: %w", path, errors.Join(outbound.ErrRepositoryContentUnavailable, err))
	}
	return content, nil
}
