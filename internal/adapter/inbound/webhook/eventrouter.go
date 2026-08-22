package webhook

import (
	"context"
	"log/slog"

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
	logger        *slog.Logger
}

func NewEventRouter(queue outbound.Queue, commands *handlecommand.UseCase, parser inboundcommand.Parser, installations outbound.InstallationRepository, logger *slog.Logger) *EventRouter {
	return &EventRouter{
		queue:         queue,
		commands:      commands,
		parser:        parser,
		installations: installations,
		logger:        logger,
	}
}

func (r *EventRouter) Route(ctx context.Context, event any) string {
	switch payload := event.(type) {
	case *gh.PullRequestEvent:
		return r.pullRequest(ctx, payload)
	case *gh.IssueCommentEvent:
		return r.issueComment(ctx, payload)
	case *gh.PullRequestReviewCommentEvent:
		return r.reviewComment(ctx, payload)
	case *gh.InstallationEvent:
		return r.installation(ctx, payload)
	case *gh.InstallationRepositoriesEvent:
		return r.installationRepositories(ctx, payload)
	default:
		return "ignored"
	}
}

func (r *EventRouter) pullRequest(ctx context.Context, payload *gh.PullRequestEvent) string {
	action := payload.GetAction()
	if payload.GetPullRequest().GetDraft() && action != "ready_for_review" {
		return action
	}
	target := pullrequest.Target{
		InstallationID: payload.GetInstallation().GetID(),
		Owner:          payload.GetRepo().GetOwner().GetLogin(),
		Repository:     payload.GetRepo().GetName(),
		Number:         payload.GetNumber(),
		HeadSHA:        payload.GetPullRequest().GetHead().GetSHA(),
		BaseSHA:        payload.GetPullRequest().GetBase().GetSHA(),
	}
	task := job.ReviewJob{Target: target}
	switch action {
	case "opened", "reopened", "ready_for_review":
		task.Trigger = review.TriggerPullRequestOpened
	case "synchronize":
		task.Trigger = review.TriggerPullRequestPushed
		task.Incremental = true
	default:
		return action
	}
	if err := r.queue.EnqueueReview(ctx, task); err != nil {
		r.logger.Error("리뷰 작업을 큐에 넣지 못했다", "target", target.Reference(), "error", err)
	}
	return action
}

func (r *EventRouter) issueComment(ctx context.Context, payload *gh.IssueCommentEvent) string {
	action := payload.GetAction()
	if action != "created" || !payload.GetIssue().IsPullRequest() || isBot(payload.GetSender()) {
		return action
	}
	parsed := r.parser.Parse(payload.GetComment().GetBody(), payload.GetSender().GetLogin(), payload.GetComment().GetID(), false)
	if !parsed.IsRecognized() {
		return action
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
	}); err != nil {
		r.logger.Error("명령을 처리하지 못했다", "target", target.Reference(), "error", err)
	}
	return action
}

func (r *EventRouter) reviewComment(ctx context.Context, payload *gh.PullRequestReviewCommentEvent) string {
	action := payload.GetAction()
	if action != "created" || isBot(payload.GetSender()) {
		return action
	}
	parsed := r.parser.Parse(payload.GetComment().GetBody(), payload.GetSender().GetLogin(), payload.GetComment().GetID(), true)
	if !parsed.IsRecognized() {
		return action
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
	}); err != nil {
		r.logger.Error("스레드 명령을 처리하지 못했다", "target", target.Reference(), "error", err)
	}
	return action
}

func (r *EventRouter) installation(ctx context.Context, payload *gh.InstallationEvent) string {
	action := payload.GetAction()
	entry := installation.Installation{
		ID:          payload.GetInstallation().GetID(),
		Account:     payload.GetInstallation().GetAccount().GetLogin(),
		AccountType: payload.GetInstallation().GetAccount().GetType(),
		Selection:   payload.GetInstallation().GetRepositorySelection(),
		InstalledAt: payload.GetInstallation().GetCreatedAt().Time,
	}
	if err := r.installations.Upsert(ctx, entry); err != nil {
		r.logger.Error("설치 정보를 저장하지 못했다", "installation", entry.ID, "error", err)
	}
	for _, repository := range payload.Repositories {
		r.saveRepository(ctx, entry.ID, repository)
	}
	return action
}

func (r *EventRouter) installationRepositories(ctx context.Context, payload *gh.InstallationRepositoriesEvent) string {
	installationID := payload.GetInstallation().GetID()
	for _, repository := range payload.RepositoriesAdded {
		r.saveRepository(ctx, installationID, repository)
	}
	return payload.GetAction()
}

func (r *EventRouter) saveRepository(ctx context.Context, installationID int64, repository *gh.Repository) {
	owner, name := splitFullName(repository.GetFullName())
	if owner == "" {
		owner = repository.GetOwner().GetLogin()
		name = repository.GetName()
	}
	if owner == "" || name == "" {
		return
	}
	entry := installation.Repository{
		InstallationID: installationID,
		Owner:          owner,
		Name:           name,
		Private:        repository.GetPrivate(),
	}
	if err := r.installations.UpsertRepository(ctx, entry); err != nil {
		r.logger.Error("저장소 정보를 저장하지 못했다", "repository", entry.FullName(), "error", err)
	}
}

func isBot(sender *gh.User) bool {
	return sender.GetType() == "Bot"
}
