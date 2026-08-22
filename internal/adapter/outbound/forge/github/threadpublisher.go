package github

import (
	"context"
	"fmt"
	"sort"

	gh "github.com/google/go-github/v90/github"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/thread"
)

type ThreadPublisher struct {
	clients *ClientFactory
}

func NewThreadPublisher(clients *ClientFactory) *ThreadPublisher {
	return &ThreadPublisher{clients: clients}
}

func (p *ThreadPublisher) Thread(ctx context.Context, target pullrequest.Target, commentID int64) (thread.Thread, error) {
	client, err := p.clients.Client(ctx, target.InstallationID)
	if err != nil {
		return thread.Thread{}, err
	}
	anchor, _, err := client.PullRequests.GetComment(ctx, target.Owner, target.Repository, commentID)
	if err != nil {
		return thread.Thread{}, fmt.Errorf("리뷰 코멘트를 읽지 못했습니다: %w", err)
	}
	rootID := anchor.GetID()
	if anchor.GetInReplyTo() != 0 {
		rootID = anchor.GetInReplyTo()
	}

	comments, err := p.listComments(ctx, client, target)
	if err != nil {
		return thread.Thread{}, err
	}
	collected := make([]*gh.PullRequestComment, 0, 8)
	var root *gh.PullRequestComment
	for _, comment := range comments {
		if comment.GetID() == rootID {
			root = comment
		}
		if comment.GetID() == rootID || comment.GetInReplyTo() == rootID {
			collected = append(collected, comment)
		}
	}
	if root == nil {
		root = anchor
		collected = append(collected, anchor)
	}
	sort.SliceStable(collected, func(left, right int) bool {
		return collected[left].GetCreatedAt().Time.Before(collected[right].GetCreatedAt().Time)
	})

	conversation := thread.Thread{
		RootCommentID: rootID,
		ReplyToID:     commentID,
		Path:          root.GetPath(),
		Line:          commentLine(root),
		DiffHunk:      root.GetDiffHunk(),
	}
	for _, comment := range collected {
		conversation.Messages = append(conversation.Messages, thread.Message{
			Author:    comment.GetUser().GetLogin(),
			Body:      comment.GetBody(),
			FromBot:   comment.GetUser().GetType() == "Bot",
			CreatedAt: comment.GetCreatedAt().Time,
		})
	}
	return conversation, nil
}

func (p *ThreadPublisher) Reply(ctx context.Context, target pullrequest.Target, commentID int64, body string) error {
	client, err := p.clients.Client(ctx, target.InstallationID)
	if err != nil {
		return err
	}
	if _, _, err := client.PullRequests.CreateCommentInReplyTo(ctx, target.Owner, target.Repository, target.Number, body, commentID); err != nil {
		return fmt.Errorf("스레드에 답글을 남기지 못했습니다: %w", err)
	}
	return nil
}

func (p *ThreadPublisher) listComments(ctx context.Context, client *gh.Client, target pullrequest.Target) ([]*gh.PullRequestComment, error) {
	options := &gh.PullRequestListCommentsOptions{ListOptions: gh.ListOptions{PerPage: 100}}
	collected := make([]*gh.PullRequestComment, 0, 64)
	for {
		comments, response, err := client.PullRequests.ListComments(ctx, target.Owner, target.Repository, target.Number, options)
		if err != nil {
			return nil, fmt.Errorf("리뷰 코멘트 목록을 읽지 못했습니다: %w", err)
		}
		collected = append(collected, comments...)
		if response == nil || response.NextPage == 0 {
			break
		}
		options.Page = response.NextPage
	}
	return collected, nil
}

func commentLine(comment *gh.PullRequestComment) int {
	if line := comment.GetLine(); line > 0 {
		return line
	}
	return comment.GetOriginalLine()
}
