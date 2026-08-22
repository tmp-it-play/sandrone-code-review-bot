package github

import (
	"context"
	"fmt"
	"strings"

	gh "github.com/google/go-github/v90/github"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

type ReviewPublisher struct {
	clients *ClientFactory
}

func NewReviewPublisher(clients *ClientFactory) *ReviewPublisher {
	return &ReviewPublisher{clients: clients}
}

func (p *ReviewPublisher) SubmitReview(ctx context.Context, target pullrequest.Target, body string, comments []review.InlineComment) error {
	client, err := p.clients.Client(ctx, target.InstallationID)
	if err != nil {
		return err
	}
	drafts := make([]*gh.DraftReviewComment, 0, len(comments))
	for _, comment := range comments {
		draft := &gh.DraftReviewComment{
			Path: gh.Ptr(comment.Path),
			Body: gh.Ptr(comment.Body),
			Line: gh.Ptr(comment.Line),
			Side: gh.Ptr("RIGHT"),
		}
		if comment.StartLine > 0 && comment.StartLine < comment.Line {
			draft.StartLine = gh.Ptr(comment.StartLine)
			draft.StartSide = gh.Ptr("RIGHT")
		}
		drafts = append(drafts, draft)
	}
	request := &gh.PullRequestReviewRequest{
		Body:     gh.Ptr(body),
		Event:    gh.Ptr("COMMENT"),
		Comments: drafts,
	}
	if target.HeadSHA != "" {
		request.CommitID = gh.Ptr(target.HeadSHA)
	}
	if _, _, err := client.PullRequests.CreateReview(ctx, target.Owner, target.Repository, target.Number, request); err != nil {
		return fmt.Errorf("리뷰를 제출하지 못했습니다: %w", err)
	}
	return nil
}

func (p *ReviewPublisher) CreateComment(ctx context.Context, target pullrequest.Target, body string) (int64, error) {
	client, err := p.clients.Client(ctx, target.InstallationID)
	if err != nil {
		return 0, err
	}
	comment, _, err := client.Issues.CreateComment(ctx, target.Owner, target.Repository, target.Number, &gh.IssueComment{Body: gh.Ptr(body)})
	if err != nil {
		return 0, fmt.Errorf("코멘트를 남기지 못했습니다: %w", err)
	}
	return comment.GetID(), nil
}

func (p *ReviewPublisher) UpdateComment(ctx context.Context, target pullrequest.Target, commentID int64, body string) error {
	client, err := p.clients.Client(ctx, target.InstallationID)
	if err != nil {
		return err
	}
	if _, _, err := client.Issues.EditComment(ctx, target.Owner, target.Repository, commentID, &gh.IssueComment{Body: gh.Ptr(body)}); err != nil {
		return fmt.Errorf("코멘트를 수정하지 못했습니다: %w", err)
	}
	return nil
}

func (p *ReviewPublisher) FindComment(ctx context.Context, target pullrequest.Target, marker string) (int64, bool, error) {
	client, err := p.clients.Client(ctx, target.InstallationID)
	if err != nil {
		return 0, false, err
	}
	options := &gh.IssueListCommentsOptions{ListOptions: gh.ListOptions{PerPage: 100}}
	found := int64(0)
	for {
		comments, response, listErr := client.Issues.ListComments(ctx, target.Owner, target.Repository, target.Number, options)
		if listErr != nil {
			return 0, false, fmt.Errorf("코멘트 목록을 읽지 못했습니다: %w", listErr)
		}
		for _, comment := range comments {
			if strings.Contains(comment.GetBody(), marker) {
				found = comment.GetID()
			}
		}
		if response == nil || response.NextPage == 0 {
			break
		}
		options.Page = response.NextPage
	}
	return found, found != 0, nil
}

func (p *ReviewPublisher) UpdatePullRequestBody(ctx context.Context, target pullrequest.Target, marker string, section string) error {
	client, err := p.clients.Client(ctx, target.InstallationID)
	if err != nil {
		return err
	}
	current, _, err := client.PullRequests.Get(ctx, target.Owner, target.Repository, target.Number)
	if err != nil {
		return fmt.Errorf("PR 본문을 읽지 못했습니다: %w", err)
	}
	updated := replaceSection(current.GetBody(), marker, section)
	if _, _, err := client.PullRequests.Edit(ctx, target.Owner, target.Repository, target.Number, &gh.PullRequest{Body: gh.Ptr(updated)}); err != nil {
		return fmt.Errorf("PR 본문을 수정하지 못했습니다: %w", err)
	}
	return nil
}

func replaceSection(body string, marker string, section string) string {
	closing := closingMarker(marker)
	start := strings.Index(body, marker)
	end := strings.Index(body, closing)
	if start < 0 || end < 0 || end < start {
		if strings.TrimSpace(body) == "" {
			return section
		}
		return strings.TrimRight(body, "\n") + "\n\n" + section
	}
	return body[:start] + section + body[end+len(closing):]
}

func closingMarker(marker string) string {
	return strings.Replace(marker, "<!-- ", "<!-- /", 1)
}
