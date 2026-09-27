package github

import (
	"context"
	"fmt"
	"time"

	gh "github.com/google/go-github/v90/github"

	"github.com/it-play/sandrone-code-review-bot/internal/core/progresscomment"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
)

const progressCheckName = "Sandrone"

type ProgressCheckPublisher struct {
	clients *ClientFactory
}

func NewProgressCheckPublisher(clients *ClientFactory) *ProgressCheckPublisher {
	return &ProgressCheckPublisher{clients: clients}
}

func (p *ProgressCheckPublisher) CreateProgressCheck(ctx context.Context, target pullrequest.Target, externalID string, title string) (int64, error) {
	client, err := p.clients.Client(ctx, target.InstallationID)
	if err != nil {
		return 0, err
	}
	created, _, err := client.Checks.CreateCheckRun(ctx, target.Owner, target.Repository, gh.CreateCheckRunOptions{
		Name:       progressCheckName,
		HeadSHA:    target.HeadSHA,
		ExternalID: gh.Ptr(externalID),
		Status:     gh.Ptr("in_progress"),
		StartedAt:  &gh.Timestamp{Time: time.Now()},
		Output:     progressCheckOutput(title),
	})
	if err != nil {
		return 0, fmt.Errorf("진행 체크를 만들지 못했습니다: %w", err)
	}
	return created.GetID(), nil
}

func (p *ProgressCheckPublisher) UpdateProgressCheck(ctx context.Context, target pullrequest.Target, checkRunID int64, title string) error {
	client, err := p.clients.Client(ctx, target.InstallationID)
	if err != nil {
		return err
	}
	if _, _, err := client.Checks.UpdateCheckRun(ctx, target.Owner, target.Repository, checkRunID, gh.UpdateCheckRunOptions{
		Name:   progressCheckName,
		Status: gh.Ptr("in_progress"),
		Output: progressCheckOutput(title),
	}); err != nil {
		return fmt.Errorf("진행 체크를 갱신하지 못했습니다: %w", err)
	}
	return nil
}

func (p *ProgressCheckPublisher) CompleteProgressCheck(ctx context.Context, target pullrequest.Target, checkRunID int64, conclusion progresscomment.CheckConclusion, title string) error {
	client, err := p.clients.Client(ctx, target.InstallationID)
	if err != nil {
		return err
	}
	if title == "" {
		title = string(conclusion)
	}
	if _, _, err := client.Checks.UpdateCheckRun(ctx, target.Owner, target.Repository, checkRunID, gh.UpdateCheckRunOptions{
		Name:        progressCheckName,
		Status:      gh.Ptr("completed"),
		Conclusion:  gh.Ptr(string(conclusion)),
		CompletedAt: &gh.Timestamp{Time: time.Now()},
		Output:      progressCheckOutput(title),
	}); err != nil {
		return fmt.Errorf("진행 체크를 완료하지 못했습니다: %w", err)
	}
	return nil
}

func progressCheckOutput(title string) *gh.CheckRunOutput {
	return &gh.CheckRunOutput{
		Title:   gh.Ptr(title),
		Summary: gh.Ptr(title),
	}
}
