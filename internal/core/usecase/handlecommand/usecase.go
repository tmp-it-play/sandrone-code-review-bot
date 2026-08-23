package handlecommand

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/command"
	"github.com/it-play/sandrone-code-review-bot/internal/core/job"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

type UseCase struct {
	deps Dependencies
}

func New(deps Dependencies) *UseCase {
	return &UseCase{deps: deps}
}

func (u *UseCase) Execute(ctx context.Context, request Request) error {
	if !request.Command.IsRecognized() {
		return nil
	}
	if request.OccurredAt.IsZero() {
		request.OccurredAt = u.deps.Clock.Now()
	}
	u.acknowledge(ctx, request)
	allowed, err := u.allowed(ctx, request)
	if err != nil {
		return err
	}
	if err := u.record(ctx, request, allowed, ""); err != nil {
		return err
	}
	if !allowed {
		return u.reject(ctx, request)
	}
	return u.enqueue(ctx, request)
}

func (u *UseCase) allowed(ctx context.Context, request Request) (bool, error) {
	if request.Command.Invoker == "" {
		return false, nil
	}
	if strings.EqualFold(request.Command.Invoker, request.PullRequestAuthor) {
		return true, nil
	}
	permission, err := u.deps.Permissions.Permission(ctx, request.Target, request.Command.Invoker)
	if err != nil {
		return false, fmt.Errorf("권한을 확인하지 못했습니다: %w", err)
	}
	return permission.CanInvoke(), nil
}

func (u *UseCase) enqueue(ctx context.Context, request Request) error {
	switch request.Command.Kind {
	case command.KindReview:
		return u.deps.Queue.EnqueueReview(ctx, job.ReviewJob{
			Target:             request.Target,
			Trigger:            review.TriggerCommandReview,
			Invoker:            request.Command.Invoker,
			Instruction:        request.Command.Instruction,
			CommentID:          request.Command.CommentID,
			InThread:           request.Command.InThread,
			RequestIdentity:    request.RequestIdentity,
			SnapshotObservedAt: commandOccurredAt(request, u.deps.Clock.Now()),
			SnapshotOrderKey:   commandOrderKey(request),
		})
	case command.KindSummary:
		return u.deps.Queue.EnqueueSummary(ctx, job.SummaryJob{
			Target:          request.Target,
			Trigger:         review.TriggerCommandSummary,
			Invoker:         request.Command.Invoker,
			Instruction:     request.Command.Instruction,
			CommentID:       request.Command.CommentID,
			InThread:        request.Command.InThread,
			RequestIdentity: request.RequestIdentity,
			OperationKey:    commandOperationKey(request),
			OrderKey:        commandOrderKey(request),
			RequestedAt:     commandOccurredAt(request, u.deps.Clock.Now()),
		})
	case command.KindReply:
		return u.deps.Queue.EnqueueReply(ctx, job.ReplyJob{
			Target:          request.Target,
			Invoker:         request.Command.Invoker,
			Instruction:     request.Command.Instruction,
			CommentID:       request.Command.CommentID,
			InThread:        request.Command.InThread,
			RequestIdentity: request.RequestIdentity,
			OperationKey:    commandOperationKey(request),
		})
	default:
		return nil
	}
}

func (u *UseCase) acknowledge(ctx context.Context, request Request) {
	if request.Command.CommentID == 0 {
		return
	}
	if err := u.deps.Reactions.AddReaction(ctx, request.Target, request.Command.CommentID, request.Command.InThread, "eyes"); err != nil {
		u.deps.Logger.Warn("리액션을 남기지 못했습니다", "target", request.Target.Reference(), "error", err)
	}
}

func (u *UseCase) reject(ctx context.Context, request Request) error {
	notice := review.Notice{
		Kind:    review.NoticeRejected,
		Message: fmt.Sprintf("@%s 님은 이 저장소에 대한 쓰기 권한이 없어 명령을 실행할 수 없습니다.", request.Command.Invoker),
	}
	marker := commandMarker(request)
	body := u.deps.Renderer.NoticeBody(notice) + "\n" + marker
	if request.Command.InThread {
		rootCommentID, exists, err := u.threadReplyExists(ctx, request, marker)
		if err != nil || exists {
			return err
		}
		replyErr := u.deps.Threads.Reply(ctx, request.Target, rootCommentID, body)
		if replyErr == nil {
			return nil
		}
		_, exists, reconcileErr := u.threadReplyExists(ctx, request, marker)
		if exists {
			return nil
		}
		return errors.Join(replyErr, reconcileErr)
	}
	_, exists, err := u.deps.Publisher.FindComment(ctx, request.Target, marker)
	if err != nil || exists {
		return err
	}
	_, createErr := u.deps.Publisher.CreateComment(ctx, request.Target, body)
	if createErr == nil {
		return nil
	}
	_, exists, reconcileErr := u.deps.Publisher.FindComment(ctx, request.Target, marker)
	if exists {
		return nil
	}
	return errors.Join(createErr, reconcileErr)
}

func (u *UseCase) threadReplyExists(ctx context.Context, request Request, marker string) (int64, bool, error) {
	conversation, err := u.deps.Threads.Thread(ctx, request.Target, request.Command.CommentID)
	if err != nil {
		return 0, false, err
	}
	for _, message := range conversation.Messages {
		if message.FromBot && strings.Contains(message.Body, marker) {
			return conversation.RootCommentID, true, nil
		}
	}
	return conversation.RootCommentID, false, nil
}

func (u *UseCase) record(ctx context.Context, request Request, allowed bool, detail string) error {
	invocation := command.Invocation{
		Key:        commandOperationKey(request),
		Owner:      request.Target.Owner,
		Repository: request.Target.Repository,
		Number:     request.Target.Number,
		Invoker:    request.Command.Invoker,
		Kind:       request.Command.Kind,
		Allowed:    allowed,
		Detail:     detail,
		OccurredAt: commandOccurredAt(request, u.deps.Clock.Now()),
	}
	if err := u.deps.Commands.Record(ctx, invocation); err != nil {
		return fmt.Errorf("명령 기록을 저장하지 못했습니다: %w", err)
	}
	return nil
}

func commandOccurredAt(request Request, fallback time.Time) time.Time {
	if request.OccurredAt.IsZero() {
		return fallback
	}
	return request.OccurredAt
}
