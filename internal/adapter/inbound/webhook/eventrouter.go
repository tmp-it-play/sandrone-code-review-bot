package webhook

import (
	"context"
	"fmt"
	"time"

	gh "github.com/google/go-github/v90/github"
	inboundcommand "github.com/it-play/sandrone-code-review-bot/internal/adapter/inbound/command"
	"github.com/it-play/sandrone-code-review-bot/internal/core/installation"
	"github.com/it-play/sandrone-code-review-bot/internal/core/job"
	"github.com/it-play/sandrone-code-review-bot/internal/core/port/outbound"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"github.com/it-play/sandrone-code-review-bot/internal/core/usecase/handlecommand"
)

type EventRouter struct {
	queue         outbound.Queue
	commands      *handlecommand.UseCase
	parser        inboundcommand.Parser
	installations outbound.InstallationRepository
}

func NewEventRouter(queue outbound.Queue, commands *handlecommand.UseCase, parser inboundcommand.Parser, installations outbound.InstallationRepository) *EventRouter {
	return &EventRouter{
		queue:         queue,
		commands:      commands,
		parser:        parser,
		installations: installations,
	}
}

func (r *EventRouter) Route(ctx context.Context, event any, requestIdentity string, receivedAt time.Time) (string, error) {
	switch payload := event.(type) {
	case *gh.PullRequestEvent:
		return r.pullRequest(ctx, payload, requestIdentity, receivedAt)
	case *gh.IssueCommentEvent:
		return r.issueComment(ctx, payload, requestIdentity)
	case *gh.PullRequestReviewCommentEvent:
		return r.reviewComment(ctx, payload, requestIdentity)
	case *gh.InstallationEvent:
		return r.installation(ctx, payload)
	case *gh.InstallationRepositoriesEvent:
		return r.installationRepositories(ctx, payload)
	default:
		return "ignored", nil
	}
}

func (r *EventRouter) pullRequest(ctx context.Context, payload *gh.PullRequestEvent, requestIdentity string, receivedAt time.Time) (string, error) {
	action := payload.GetAction()
	draft := payload.GetPullRequest().GetDraft()
	if draft && action != "opened" && action != "ready_for_review" {
		return action, nil
	}
	target := pullrequest.Target{
		InstallationID: payload.GetInstallation().GetID(),
		Owner:          payload.GetRepo().GetOwner().GetLogin(),
		Repository:     payload.GetRepo().GetName(),
		Number:         payload.GetNumber(),
		HeadSHA:        payload.GetPullRequest().GetHead().GetSHA(),
		BaseSHA:        payload.GetPullRequest().GetBase().GetSHA(),
	}
	task := job.ReviewJob{
		Target:             target,
		RequestIdentity:    requestIdentity,
		RequestReceivedAt:  receivedAt,
		SnapshotObservedAt: payload.GetPullRequest().GetUpdatedAt().Time,
		SnapshotOrderKey:   "automatic",
	}
	switch action {
	case "opened":
		if draft {
			task.Trigger = review.TriggerPullRequestDraftOpened
		} else {
			task.Trigger = review.TriggerPullRequestOpened
		}
	case "reopened", "ready_for_review":
		task.Trigger = review.TriggerPullRequestOpened
	case "synchronize":
		task.Trigger = review.TriggerPullRequestPushed
		task.Incremental = true
		task.PreviousHeadSHA = payload.GetBefore()
	default:
		return action, nil
	}
	if err := r.queue.EnqueueReview(ctx, task); err != nil {
		return action, fmt.Errorf("%s 리뷰 작업을 큐에 넣지 못했습니다: %w", target.Reference(), err)
	}
	return action, nil
}

func (r *EventRouter) issueComment(ctx context.Context, payload *gh.IssueCommentEvent, requestIdentity string) (string, error) {
	action := payload.GetAction()
	if action != "created" || !payload.GetIssue().IsPullRequest() || isBot(payload.GetSender()) {
		return action, nil
	}
	parsed := r.parser.Parse(payload.GetComment().GetBody(), payload.GetSender().GetLogin(), payload.GetComment().GetID(), false)
	if !parsed.IsRecognized() {
		return action, nil
	}
	target := pullrequest.Target{
		InstallationID: payload.GetInstallation().GetID(),
		Owner:          payload.GetRepo().GetOwner().GetLogin(),
		Repository:     payload.GetRepo().GetName(),
		Number:         payload.GetIssue().GetNumber(),
	}
	if err := r.commands.Execute(ctx, handlecommand.Request{
		Target:            target,
		Command:           parsed,
		PullRequestAuthor: payload.GetIssue().GetUser().GetLogin(),
		RequestIdentity:   requestIdentity,
		OccurredAt:        payload.GetComment().GetCreatedAt().Time,
	}); err != nil {
		return action, fmt.Errorf("%s 명령을 처리하지 못했습니다: %w", target.Reference(), err)
	}
	return action, nil
}

func (r *EventRouter) reviewComment(ctx context.Context, payload *gh.PullRequestReviewCommentEvent, requestIdentity string) (string, error) {
	action := payload.GetAction()
	if action != "created" || isBot(payload.GetSender()) {
		return action, nil
	}
	parsed := r.parser.Parse(payload.GetComment().GetBody(), payload.GetSender().GetLogin(), payload.GetComment().GetID(), true)
	if !parsed.IsRecognized() {
		return action, nil
	}
	target := pullrequest.Target{
		InstallationID: payload.GetInstallation().GetID(),
		Owner:          payload.GetRepo().GetOwner().GetLogin(),
		Repository:     payload.GetRepo().GetName(),
		Number:         payload.GetPullRequest().GetNumber(),
		HeadSHA:        payload.GetPullRequest().GetHead().GetSHA(),
	}
	if err := r.commands.Execute(ctx, handlecommand.Request{
		Target:            target,
		Command:           parsed,
		PullRequestAuthor: payload.GetPullRequest().GetUser().GetLogin(),
		RequestIdentity:   requestIdentity,
		OccurredAt:        payload.GetComment().GetCreatedAt().Time,
	}); err != nil {
		return action, fmt.Errorf("%s 스레드 명령을 처리하지 못했습니다: %w", target.Reference(), err)
	}
	return action, nil
}

func (r *EventRouter) installation(ctx context.Context, payload *gh.InstallationEvent) (string, error) {
	action := payload.GetAction()
	entry := installation.Installation{
		ID:          payload.GetInstallation().GetID(),
		Account:     payload.GetInstallation().GetAccount().GetLogin(),
		AccountType: payload.GetInstallation().GetAccount().GetType(),
		Selection:   payload.GetInstallation().GetRepositorySelection(),
		InstalledAt: payload.GetInstallation().GetCreatedAt().Time,
	}
	if err := r.installations.Upsert(ctx, entry); err != nil {
		return action, fmt.Errorf("%d 설치 정보를 저장하지 못했습니다: %w", entry.ID, err)
	}
	for _, repository := range payload.Repositories {
		if err := r.saveRepository(ctx, entry.ID, repository); err != nil {
			return action, err
		}
	}
	return action, nil
}

func (r *EventRouter) installationRepositories(ctx context.Context, payload *gh.InstallationRepositoriesEvent) (string, error) {
	installationID := payload.GetInstallation().GetID()
	for _, repository := range payload.RepositoriesAdded {
		if err := r.saveRepository(ctx, installationID, repository); err != nil {
			return payload.GetAction(), err
		}
	}
	return payload.GetAction(), nil
}

func (r *EventRouter) saveRepository(ctx context.Context, installationID int64, repository *gh.Repository) error {
	owner, name := splitFullName(repository.GetFullName())
	if owner == "" {
		owner = repository.GetOwner().GetLogin()
		name = repository.GetName()
	}
	if owner == "" || name == "" {
		return nil
	}
	entry := installation.Repository{
		InstallationID: installationID,
		Owner:          owner,
		Name:           name,
		Private:        repository.GetPrivate(),
	}
	if err := r.installations.UpsertRepository(ctx, entry); err != nil {
		return fmt.Errorf("%s 저장소 정보를 저장하지 못했습니다: %w", entry.FullName(), err)
	}
	return nil
}

func isBot(sender *gh.User) bool {
	return sender.GetType() == "Bot"
}
