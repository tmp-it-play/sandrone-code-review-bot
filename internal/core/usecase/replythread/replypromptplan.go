package replythread

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
	"github.com/it-play/sandrone-code-review-bot/internal/core/prompt"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/setting"
	"github.com/it-play/sandrone-code-review-bot/internal/core/thread"
)

type replyPromptPlan struct {
	Messages []llm.Message
}

func (u *UseCase) buildReplyPromptPlan(request pullrequest.PullRequest, conversation thread.Thread, source string, sourceTruncated bool, config setting.RepoConfig, extra string, completionRequest llm.Request) (replyPromptPlan, error) {
	budget := u.deps.Completer.PromptBudgetFor(completionRequest)
	if budget <= 0 {
		return replyPromptPlan{}, fmt.Errorf("답글에 사용할 수 있는 LLM 입력 한도가 없습니다")
	}
	if config.MaxPromptChars > 0 && config.MaxPromptChars < budget {
		budget = config.MaxPromptChars
	}
	originalMessages := append([]thread.Message{}, conversation.Messages...)
	bounded := conversation
	bounded.Messages = nil
	if len(originalMessages) > 0 {
		bounded.Messages = []thread.Message{originalMessages[len(originalMessages)-1]}
	}
	omitted := len(originalMessages) - len(bounded.Messages)
	if !u.replyMessagesFit(request, bounded, source, sourceTruncated, config, replyPlanExtra(extra, omitted), budget) {
		bounded.DiffHunk = ""
	}
	if !u.replyMessagesFit(request, bounded, source, sourceTruncated, config, replyPlanExtra(extra, omitted), budget) {
		extra = fitReplyText(extra, func(candidate string) bool {
			return u.replyMessagesFit(request, bounded, source, sourceTruncated, config, replyPlanExtra(candidate, omitted), budget)
		})
	}
	if !u.replyMessagesFit(request, bounded, source, sourceTruncated, config, replyPlanExtra(extra, omitted), budget) {
		source = fitReplyText(source, func(candidate string) bool {
			return u.replyMessagesFit(request, bounded, candidate, true, config, replyPlanExtra(extra, omitted), budget)
		})
		sourceTruncated = true
	}
	if !u.replyMessagesFit(request, bounded, source, sourceTruncated, config, replyPlanExtra(extra, omitted), budget) && len(bounded.Messages) > 0 {
		latest := bounded.Messages[0]
		latest.Body = fitReplyText(latest.Body, func(candidate string) bool {
			trial := bounded
			message := latest
			message.Body = candidate
			trial.Messages = []thread.Message{message}
			return u.replyMessagesFit(request, trial, source, sourceTruncated, config, replyPlanExtra(extra, omitted), budget)
		})
		bounded.Messages = []thread.Message{latest}
	}
	if !u.replyMessagesFit(request, bounded, source, sourceTruncated, config, replyPlanExtra(extra, omitted), budget) {
		return replyPromptPlan{}, fmt.Errorf("최신 대화와 현재 소스가 답글 입력 한도 %d자를 넘었습니다", budget)
	}
	for index := len(originalMessages) - 2; index >= 0; index-- {
		candidate := bounded
		candidate.Messages = append([]thread.Message{originalMessages[index]}, bounded.Messages...)
		candidateOmitted := len(originalMessages) - len(candidate.Messages)
		if !u.replyMessagesFit(request, candidate, source, sourceTruncated, config, replyPlanExtra(extra, candidateOmitted), budget) {
			continue
		}
		bounded = candidate
		omitted = candidateOmitted
	}
	messages := u.replyMessages(request, bounded, source, sourceTruncated, config, replyPlanExtra(extra, omitted))
	if llm.MessagesSize(messages) > budget {
		return replyPromptPlan{}, fmt.Errorf("답글 프롬프트가 입력 한도 %d자를 넘었습니다", budget)
	}
	return replyPromptPlan{Messages: messages}, nil
}

func (u *UseCase) replyMessages(request pullrequest.PullRequest, conversation thread.Thread, source string, truncated bool, config setting.RepoConfig, extra string) []llm.Message {
	messages := prompt.ReplyPrompt{
		PullRequest:   request,
		Thread:        conversation,
		CurrentSource: source,
		Truncated:     truncated,
		Config:        config,
		Extra:         extra,
	}.Messages()
	return llm.MaskMessages(messages, u.deps.Masker.Mask)
}

func (u *UseCase) replyMessagesFit(request pullrequest.PullRequest, conversation thread.Thread, source string, truncated bool, config setting.RepoConfig, extra string, budget int) bool {
	return llm.MessagesSize(u.replyMessages(request, conversation, source, truncated, config, extra)) <= budget
}

func replyPlanExtra(extra string, omitted int) string {
	parts := make([]string, 0, 2)
	if omitted > 0 {
		parts = append(parts, fmt.Sprintf("[입력 한도로 이전 대화 %d개 생략]", omitted))
	}
	if trimmed := strings.TrimSpace(extra); trimmed != "" {
		parts = append(parts, trimmed)
	}
	return strings.Join(parts, "\n")
}

func fitReplyText(value string, fits func(string) bool) string {
	if fits(value) {
		return value
	}
	low := 0
	high := len(value)
	best := ""
	for low <= high {
		middle := low + (high-low)/2
		candidate := truncateReplyUTF8(value, middle)
		if candidate != "" {
			candidate += "\n[입력 한도로 이후 내용 생략]"
		}
		if fits(candidate) {
			best = candidate
			low = middle + 1
		} else {
			high = middle - 1
		}
	}
	return best
}

func truncateReplyUTF8(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(value) <= limit {
		return value
	}
	truncated := value[:limit]
	for truncated != "" && !utf8.ValidString(truncated) {
		truncated = truncated[:len(truncated)-1]
	}
	return truncated
}
