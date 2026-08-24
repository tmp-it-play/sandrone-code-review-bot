package reviewworkflow

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

type ReviewPublicationPayload struct {
	Body         string
	Comments     []review.InlineComment
	FallbackBody string
}

const reviewPublicationBodyLimit = 60 * 1024

func (p ReviewPublicationPayload) Bounded(marker string) ReviewPublicationPayload {
	if p.Comments == nil {
		p.Comments = make([]review.InlineComment, 0)
	}
	p.Body = boundedPublicationBody(p.Body, marker)
	p.FallbackBody = boundedPublicationBody(p.FallbackBody, marker)
	return p
}

func (p ReviewPublicationPayload) Validate(marker string) error {
	if strings.TrimSpace(marker) == "" {
		return errors.New("리뷰 게시 marker가 비어 있습니다")
	}
	if strings.TrimSpace(p.Body) == "" || !strings.Contains(p.Body, marker) {
		return errors.New("리뷰 게시 본문에 marker가 없습니다")
	}
	if strings.TrimSpace(p.FallbackBody) == "" || !strings.Contains(p.FallbackBody, marker) {
		return errors.New("리뷰 게시 fallback 본문에 marker가 없습니다")
	}
	if len(p.Body) > reviewPublicationBodyLimit || len(p.FallbackBody) > reviewPublicationBodyLimit {
		return errors.New("리뷰 게시 본문이 허용 크기를 넘었습니다")
	}
	for _, comment := range p.Comments {
		if strings.TrimSpace(comment.Path) == "" || strings.TrimSpace(comment.Body) == "" || comment.Line <= 0 {
			return errors.New("인라인 리뷰 코멘트가 올바르지 않습니다")
		}
		if comment.StartLine != 0 && (comment.StartLine < 1 || comment.StartLine >= comment.Line) {
			return errors.New("인라인 리뷰 코멘트 범위가 올바르지 않습니다")
		}
		if len(comment.Body) > reviewPublicationBodyLimit {
			return errors.New("인라인 리뷰 코멘트가 허용 크기를 넘었습니다")
		}
	}
	return nil
}

func boundedPublicationBody(body string, marker string) string {
	if len(body) <= reviewPublicationBodyLimit {
		return body
	}
	markerIndex := strings.LastIndex(body, marker)
	if markerIndex < 0 {
		return body
	}
	suffix := "\n\n_게시 본문이 길어 일부를 생략했습니다._\n" + marker
	limit := reviewPublicationBodyLimit - len(suffix)
	if limit < 0 {
		return body
	}
	prefix := strings.TrimRight(body[:markerIndex], "\n")
	if len(prefix) > limit {
		prefix = prefix[:limit]
		for len(prefix) > 0 && !utf8.ValidString(prefix) {
			prefix = prefix[:len(prefix)-1]
		}
	}
	return strings.TrimRight(prefix, "\n") + suffix
}

func (p ReviewPublicationPayload) Hash() (string, error) {
	comments, err := json.Marshal(p.Comments)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	for _, part := range []string{"review-publication-v2", p.Body, string(comments), p.FallbackBody} {
		hash.Write([]byte(strconv.Itoa(len(part))))
		hash.Write([]byte{0})
		hash.Write([]byte(part))
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
