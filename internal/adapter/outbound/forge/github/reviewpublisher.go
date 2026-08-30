package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	gh "github.com/google/go-github/v90/github"
	"github.com/it-play/sandrone-code-review-bot/internal/core/publication"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

type ReviewPublisher struct {
	clients *ClientFactory
}

func NewReviewPublisher(clients *ClientFactory) *ReviewPublisher {
	return &ReviewPublisher{clients: clients}
}

func (p *ReviewPublisher) VerifyTarget(ctx context.Context, target pullrequest.Target) (pullrequest.Target, error) {
	client, err := p.clients.Client(ctx, target.InstallationID)
	if err != nil {
		return target, err
	}
	return verifyTarget(ctx, client, target)
}

func (p *ReviewPublisher) SubmitReview(ctx context.Context, target pullrequest.Target, marker string, body string, comments []review.InlineComment) (int64, error) {
	client, err := p.clients.Client(ctx, target.InstallationID)
	if err != nil {
		return 0, err
	}
	reviewID, exists, err := findReview(ctx, client, target, marker)
	if err != nil {
		return 0, err
	}
	if exists {
		verifiedTarget, verifyErr := verifyTarget(ctx, client, target)
		if verifyErr != nil {
			return 0, verifyErr
		}
		target = verifiedTarget
		if _, _, err := client.PullRequests.UpdateReview(ctx, target.Owner, target.Repository, target.Number, reviewID, review.SanitizePublicBody(body)); err != nil {
			return 0, fmt.Errorf("기존 리뷰 본문을 갱신하지 못했습니다: %w", withGitHubRetryAt(err))
		}
		return reviewID, nil
	}
	verifiedTarget, verifyErr := verifyTarget(ctx, client, target)
	if verifyErr != nil {
		return 0, verifyErr
	}
	target = verifiedTarget
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
		Body:     gh.Ptr(review.SanitizePublicBody(body)),
		Event:    gh.Ptr("COMMENT"),
		Comments: drafts,
	}
	if target.HeadSHA != "" {
		request.CommitID = gh.Ptr(target.HeadSHA)
	}
	created, _, err := client.PullRequests.CreateReview(ctx, target.Owner, target.Repository, target.Number, request)
	if err != nil {
		createErr := fmt.Errorf("리뷰를 제출하지 못했습니다: %w", withGitHubRetryAt(err))
		reconciledID, exists, reconcileErr := findReview(ctx, client, target, marker)
		if reconcileErr == nil && exists {
			return reconciledID, nil
		}
		if reconcileErr == nil && inlineReviewRejected(err) {
			return 0, errors.Join(publication.ErrInlineReviewRejected, createErr)
		}
		return 0, errors.Join(createErr, reconcileErr)
	}
	return created.GetID(), nil
}

func inlineReviewRejected(err error) bool {
	var responseError *gh.ErrorResponse
	if !errors.As(err, &responseError) || responseError.Response == nil || responseError.Response.StatusCode != http.StatusUnprocessableEntity {
		return false
	}
	if !retryAfterFromResponse(responseError.Response, time.Now()).IsZero() {
		return false
	}
	detail := strings.ToLower(responseError.GetMessage())
	for _, item := range responseError.Errors {
		detail += " " + strings.ToLower(item.GetMessage())
	}
	if strings.Contains(detail, "spam") || strings.Contains(detail, "secondary rate limit") || strings.Contains(detail, "abuse") || strings.Contains(detail, "rate limit") {
		return false
	}
	for _, item := range responseError.Errors {
		resource := strings.ToLower(item.GetResource())
		field := strings.ToLower(item.GetField())
		code := strings.ToLower(item.GetCode())
		reviewResource := strings.Contains(resource, "pullrequestreview") || strings.Contains(resource, "pull request review")
		inlineField := field == "comments" || field == "path" || field == "position" || field == "line" || field == "start_line" || field == "side" || field == "start_side" || field == "commit_id"
		validationCode := code == "invalid" || code == "missing" || code == "missing_field" || code == "unprocessable" || code == "custom"
		if validationCode && (reviewResource || inlineField) {
			return true
		}
	}
	return false
}

func verifyTarget(ctx context.Context, client *gh.Client, target pullrequest.Target) (pullrequest.Target, error) {
	current, _, err := client.PullRequests.Get(ctx, target.Owner, target.Repository, target.Number)
	if err != nil {
		return target, fmt.Errorf("리뷰 게시 직전 PR 상태를 읽지 못했습니다: %w", withGitHubRetryAt(err))
	}
	currentHeadSHA := current.GetHead().GetSHA()
	currentBaseSHA := current.GetBase().GetSHA()
	if target.BaseSHA != "" && currentBaseSHA != target.BaseSHA {
		return target, publication.ErrTargetChanged
	}
	if target.HeadSHA != "" && currentHeadSHA != target.HeadSHA {
		files, compareErr := changedFilesBetween(ctx, client, target, target.HeadSHA, currentHeadSHA)
		if compareErr != nil {
			return target, compareErr
		}
		if len(files) != 0 {
			return target, publication.ErrTargetChanged
		}
	}
	target.HeadSHA = currentHeadSHA
	target.BaseSHA = currentBaseSHA
	target.BaseRef = current.GetBase().GetRef()
	return target, nil
}

func (p *ReviewPublisher) PublicationExists(ctx context.Context, target pullrequest.Target, marker string) (bool, error) {
	client, err := p.clients.Client(ctx, target.InstallationID)
	if err != nil {
		return false, err
	}
	exists, err := reviewExists(ctx, client, target, marker)
	if err != nil || exists {
		return exists, err
	}
	_, exists, err = findComment(ctx, client, target, marker)
	return exists, err
}

func (p *ReviewPublisher) InvalidatePublication(ctx context.Context, target pullrequest.Target, marker string, reason string) error {
	client, err := p.clients.Client(ctx, target.InstallationID)
	if err != nil {
		return err
	}
	return invalidateMarker(ctx, client, target, marker, reason)
}

func (p *ReviewPublisher) InvalidateReview(ctx context.Context, target pullrequest.Target, marker string, reason string) error {
	client, err := p.clients.Client(ctx, target.InstallationID)
	if err != nil {
		return err
	}
	return invalidateReviewMarker(ctx, client, target, marker, reason)
}

func invalidateMarker(ctx context.Context, client *gh.Client, target pullrequest.Target, marker string, reason string) error {
	cleanupErr := invalidateReviewMarker(ctx, client, target, marker, reason)
	invalidBody := strings.TrimSpace(reason) + "\n\n" + marker
	commentIDs, err := findComments(ctx, client, target, marker)
	if err != nil {
		cleanupErr = errors.Join(cleanupErr, err)
	} else {
		for _, commentID := range commentIDs {
			if _, _, editErr := client.Issues.EditComment(ctx, target.Owner, target.Repository, commentID, &gh.IssueComment{Body: gh.Ptr(invalidBody)}); editErr != nil {
				cleanupErr = errors.Join(cleanupErr, fmt.Errorf("오래된 리뷰 코멘트 %d를 무효화하지 못했습니다: %w", commentID, editErr))
			}
		}
	}
	return cleanupErr
}

func invalidateReviewMarker(ctx context.Context, client *gh.Client, target pullrequest.Target, marker string, reason string) error {
	invalidBody := strings.TrimSpace(reason) + "\n\n" + marker
	items, err := findReviews(ctx, client, target, marker)
	if err != nil {
		return err
	}
	var cleanupErr error
	for _, item := range items {
		comments, listErr := reviewComments(ctx, client, target, item.GetID())
		if listErr != nil {
			cleanupErr = errors.Join(cleanupErr, listErr)
		} else {
			for _, comment := range comments {
				if comment.GetInReplyTo() != 0 || comment.GetUser().GetType() != "Bot" {
					continue
				}
				if _, _, editErr := client.PullRequests.EditComment(ctx, target.Owner, target.Repository, comment.GetID(), &gh.PullRequestComment{Body: gh.Ptr(invalidBody)}); editErr != nil {
					cleanupErr = errors.Join(cleanupErr, fmt.Errorf("오래된 인라인 리뷰 코멘트 %d를 무효화하지 못했습니다: %w", comment.GetID(), editErr))
				}
			}
		}
		if _, _, updateErr := client.PullRequests.UpdateReview(ctx, target.Owner, target.Repository, target.Number, item.GetID(), invalidBody); updateErr != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("오래된 리뷰 본문을 무효화하지 못했습니다: %w", updateErr))
		}
	}
	return cleanupErr
}

func (p *ReviewPublisher) CreateComment(ctx context.Context, target pullrequest.Target, body string) (int64, error) {
	client, err := p.clients.Client(ctx, target.InstallationID)
	if err != nil {
		return 0, err
	}
	comment, _, err := client.Issues.CreateComment(ctx, target.Owner, target.Repository, target.Number, &gh.IssueComment{Body: gh.Ptr(review.SanitizePublicBody(body))})
	if err != nil {
		return 0, fmt.Errorf("코멘트를 남기지 못했습니다: %w", withGitHubRetryAt(err))
	}
	return comment.GetID(), nil
}

func (p *ReviewPublisher) UpdateComment(ctx context.Context, target pullrequest.Target, commentID int64, body string) error {
	client, err := p.clients.Client(ctx, target.InstallationID)
	if err != nil {
		return err
	}
	if _, _, err := client.Issues.EditComment(ctx, target.Owner, target.Repository, commentID, &gh.IssueComment{Body: gh.Ptr(review.SanitizePublicBody(body))}); err != nil {
		return fmt.Errorf("코멘트를 수정하지 못했습니다: %w", withGitHubRetryAt(err))
	}
	return nil
}

func (p *ReviewPublisher) FindComment(ctx context.Context, target pullrequest.Target, marker string) (int64, bool, error) {
	client, err := p.clients.Client(ctx, target.InstallationID)
	if err != nil {
		return 0, false, err
	}
	return findComment(ctx, client, target, marker)
}

func (p *ReviewPublisher) FindComments(ctx context.Context, target pullrequest.Target, marker string) ([]int64, error) {
	client, err := p.clients.Client(ctx, target.InstallationID)
	if err != nil {
		return nil, err
	}
	return findComments(ctx, client, target, marker)
}

func reviewExists(ctx context.Context, client *gh.Client, target pullrequest.Target, marker string) (bool, error) {
	items, err := findReviews(ctx, client, target, marker)
	return len(items) > 0, err
}

func findReview(ctx context.Context, client *gh.Client, target pullrequest.Target, marker string) (int64, bool, error) {
	items, err := findReviews(ctx, client, target, marker)
	if err != nil || len(items) == 0 {
		return 0, false, err
	}
	return items[len(items)-1].GetID(), true, nil
}

func findReviews(ctx context.Context, client *gh.Client, target pullrequest.Target, marker string) ([]*gh.PullRequestReview, error) {
	options := &gh.ListOptions{PerPage: 100}
	found := make([]*gh.PullRequestReview, 0)
	for {
		reviews, response, err := client.PullRequests.ListReviews(ctx, target.Owner, target.Repository, target.Number, options)
		if err != nil {
			return nil, fmt.Errorf("리뷰 목록을 읽지 못했습니다: %w", err)
		}
		for _, item := range reviews {
			if item.GetUser().GetType() == "Bot" && strings.Contains(item.GetBody(), marker) {
				found = append(found, item)
			}
		}
		if response == nil || response.NextPage == 0 {
			return found, nil
		}
		options.Page = response.NextPage
	}
}

func reviewComments(ctx context.Context, client *gh.Client, target pullrequest.Target, reviewID int64) ([]*gh.PullRequestComment, error) {
	options := &gh.ListOptions{PerPage: 100}
	collected := make([]*gh.PullRequestComment, 0)
	for {
		comments, response, err := client.PullRequests.ListReviewComments(ctx, target.Owner, target.Repository, target.Number, reviewID, options)
		if err != nil {
			return nil, fmt.Errorf("리뷰 코멘트 목록을 읽지 못했습니다: %w", err)
		}
		collected = append(collected, comments...)
		if response == nil || response.NextPage == 0 {
			return collected, nil
		}
		options.Page = response.NextPage
	}
}

func findComment(ctx context.Context, client *gh.Client, target pullrequest.Target, marker string) (int64, bool, error) {
	comments, err := findComments(ctx, client, target, marker)
	if err != nil || len(comments) == 0 {
		return 0, false, err
	}
	found := comments[len(comments)-1]
	return found, true, nil
}

func findComments(ctx context.Context, client *gh.Client, target pullrequest.Target, marker string) ([]int64, error) {
	options := &gh.IssueListCommentsOptions{ListOptions: gh.ListOptions{PerPage: 100}}
	found := make([]int64, 0)
	for {
		comments, response, listErr := client.Issues.ListComments(ctx, target.Owner, target.Repository, target.Number, options)
		if listErr != nil {
			return nil, fmt.Errorf("코멘트 목록을 읽지 못했습니다: %w", withGitHubRetryAt(listErr))
		}
		for _, comment := range comments {
			if comment.GetUser().GetType() == "Bot" && strings.Contains(comment.GetBody(), marker) {
				found = append(found, comment.GetID())
			}
		}
		if response == nil || response.NextPage == 0 {
			break
		}
		options.Page = response.NextPage
	}
	return found, nil
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
	updated := replaceSection(current.GetBody(), marker, review.SanitizePublicBody(section))
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
