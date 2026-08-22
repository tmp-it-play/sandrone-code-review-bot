package github

import (
	"context"
	"fmt"

	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
)

type ReactionPublisher struct {
	clients *ClientFactory
}

func NewReactionPublisher(clients *ClientFactory) *ReactionPublisher {
	return &ReactionPublisher{clients: clients}
}

func (p *ReactionPublisher) AddReaction(ctx context.Context, target pullrequest.Target, commentID int64, inThread bool, reaction string) error {
	client, err := p.clients.Client(ctx, target.InstallationID)
	if err != nil {
		return err
	}
	if inThread {
		_, _, err = client.Reactions.CreatePullRequestCommentReaction(ctx, target.Owner, target.Repository, commentID, reaction)
	} else {
		_, _, err = client.Reactions.CreateIssueCommentReaction(ctx, target.Owner, target.Repository, commentID, reaction)
	}
	if err != nil {
		return fmt.Errorf("리액션을 남기지 못했다: %w", err)
	}
	return nil
}
