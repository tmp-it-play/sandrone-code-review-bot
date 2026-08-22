package replythread

import (
	"context"
	"fmt"
	"strings"

	"github.com/it-play/sandrone-code-review-bot/internal/core/job"
	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
	"github.com/it-play/sandrone-code-review-bot/internal/core/prompt"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

type UseCase struct {
	deps Dependencies
}

func New(deps Dependencies) *UseCase {
	return &UseCase{deps: deps}
}

func (u *UseCase) Execute(ctx context.Context, task job.ReplyJob) error {
	config, err := u.deps.Settings.RepoConfig(ctx, task.Target)
	if err != nil {
		return u.fail(ctx, task, "설정을 읽지 못했습니다", err)
	}
	if !config.ThreadReply {
		return nil
	}
	conversation, err := u.deps.Threads.Thread(ctx, task.Target, task.CommentID)
	if err != nil {
		return u.fail(ctx, task, "리뷰 스레드를 읽지 못했습니다", err)
	}

	target := task.Target
	request, err := u.deps.Source.PullRequest(ctx, target)
	if err != nil {
		return u.fail(ctx, task, "Pull Request를 읽지 못했습니다", err)
	}
	if target.HeadSHA == "" {
		target.HeadSHA = request.HeadSHA
	}

	source, truncated := u.currentSource(ctx, task, conversation.Path, target.HeadSHA, config.MaxSourceChars)
	messages := prompt.ReplyPrompt{
		PullRequest:   request,
		Thread:        conversation,
		CurrentSource: source,
		Truncated:     truncated,
		Config:        config,
		Extra:         task.Instruction,
	}.Messages()

	response, err := u.deps.Completer.Complete(ctx, llm.Request{
		Messages:        messages,
		Temperature:     config.Temperature,
		MaxOutputTokens: config.MaxOutputTokens,
		Providers:       config.Sandrone.Providers,
	}, nil)
	if err != nil {
		return u.fail(ctx, task, "답글 모델을 호출하지 못했습니다", err)
	}
	body := strings.TrimSpace(response.Content)
	if body == "" {
		return u.fail(ctx, task, "모델이 답글을 만들지 못했습니다", fmt.Errorf("빈 응답"))
	}
	if err := u.deps.Threads.Reply(ctx, task.Target, task.CommentID, u.deps.Renderer.ReplyBody(body, review.Attribution{
		Provider:         response.Provider,
		Model:            response.Model,
		Label:            response.ModelLabel,
		PromptTokens:     response.Usage.PromptTokens,
		CompletionTokens: response.Usage.CompletionTokens,
		TotalTokens:      response.Usage.TotalTokens,
	})); err != nil {
		return u.fail(ctx, task, "답글을 남기지 못했습니다", err)
	}
	return nil
}

func (u *UseCase) currentSource(ctx context.Context, task job.ReplyJob, path string, ref string, limit int) (string, bool) {
	if strings.TrimSpace(path) == "" {
		return "", false
	}
	content, err := u.deps.Source.FileContent(ctx, task.Target, path, ref)
	if err != nil {
		u.deps.Logger.Warn("최신 파일 내용을 읽지 못했습니다", "target", task.Target.Reference(), "path", path, "error", err)
		return "", false
	}
	content = u.deps.Masker.Mask(content)
	if limit > 0 && len(content) > limit {
		return content[:limit], true
	}
	return content, false
}

func (u *UseCase) fail(ctx context.Context, task job.ReplyJob, message string, cause error) error {
	u.deps.Logger.Error(message, "target", task.Target.Reference(), "error", cause)
	var notice review.Notice
	switch {
	case task.FinalAttempt:
		notice = review.Notice{Kind: review.NoticeFailed, Message: message + " 재시도했지만 해결되지 않아 중단합니다."}
	case task.Attempt == 0:
		notice = review.Notice{Kind: review.NoticeRetrying, Message: message + " 잠시 후 다시 시도합니다."}
	}
	if notice.Kind != "" {
		body := u.deps.Renderer.NoticeBody(notice)
		if err := u.deps.Threads.Reply(ctx, task.Target, task.CommentID, body); err != nil {
			u.deps.Logger.Warn("실패 안내를 남기지 못했습니다", "target", task.Target.Reference(), "error", err)
		}
	}
	return fmt.Errorf("%s: %w", message, cause)
}
