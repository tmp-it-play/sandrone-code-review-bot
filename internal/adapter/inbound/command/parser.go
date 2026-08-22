package command

import (
	"strings"

	core "github.com/it-play/sandrone-code-review-bot/internal/core/command"
)

const (
	prefixReview      = "/pr-review"
	prefixSummary     = "/pr-summary"
	prefixReviewReply = "/pr-review-reply"
)

type Parser struct {
	mention string
}

func NewParser(botName string) Parser {
	return Parser{mention: "@" + strings.ToLower(botName)}
}

func (p Parser) Parse(body string, invoker string, commentID int64, inThread bool) core.Command {
	normalized := strings.ToLower(body)
	parsed := core.Command{
		Kind:      core.KindUnknown,
		Invoker:   invoker,
		CommentID: commentID,
		InThread:  inThread,
	}
	switch {
	case strings.Contains(normalized, prefixReviewReply):
		if !inThread {
			return parsed
		}
		parsed.Kind = core.KindReply
		parsed.Instruction = strip(body, prefixReviewReply, p.mention)
	case strings.Contains(normalized, prefixSummary):
		if inThread {
			return parsed
		}
		parsed.Kind = core.KindSummary
		parsed.Instruction = strip(body, prefixSummary, p.mention)
	case strings.Contains(normalized, prefixReview):
		if inThread {
			parsed.Kind = core.KindReply
		} else {
			parsed.Kind = core.KindReview
		}
		parsed.Instruction = strip(body, prefixReview, p.mention)
	case p.mentioned(normalized):
		if inThread {
			parsed.Kind = core.KindReply
		} else {
			parsed.Kind = core.KindReview
		}
		parsed.Instruction = strip(body, p.mention)
	}
	return parsed
}

func (p Parser) mentioned(normalized string) bool {
	return strings.Contains(normalized, p.mention)
}

func strip(body string, tokens ...string) string {
	stripped := body
	for _, token := range tokens {
		for {
			index := strings.Index(strings.ToLower(stripped), token)
			if index < 0 {
				break
			}
			stripped = stripped[:index] + stripped[index+len(token):]
		}
	}
	return strings.TrimSpace(stripped)
}
