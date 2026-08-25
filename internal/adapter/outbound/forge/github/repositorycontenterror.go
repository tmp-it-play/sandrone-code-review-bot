package github

import (
	"errors"
	"fmt"
	"net/http"

	gh "github.com/google/go-github/v90/github"
	"github.com/it-play/sandrone-code-review-bot/internal/core/port/outbound"
)

func wrapRepositoryContentError(path string, err error) error {
	var responseError *gh.ErrorResponse
	if errors.As(err, &responseError) && responseError.Response != nil && responseError.Response.StatusCode == http.StatusNotFound {
		return fmt.Errorf("%s 파일을 읽지 못했습니다: %w", path, errors.Join(outbound.ErrRepositoryContentNotFound, err))
	}
	return fmt.Errorf("%s 파일을 읽지 못했습니다: %w", path, err)
}
