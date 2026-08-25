package github

import (
	"context"
	"fmt"

	gh "github.com/google/go-github/v90/github"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
)

type RepositoryContent struct {
	clients *ClientFactory
}

func NewRepositoryContent(clients *ClientFactory) *RepositoryContent {
	return &RepositoryContent{clients: clients}
}

func (c *RepositoryContent) File(ctx context.Context, target pullrequest.Target, path string, ref string) (string, error) {
	client, err := c.clients.Client(ctx, target.InstallationID)
	if err != nil {
		return "", err
	}
	file, _, _, err := client.Repositories.GetContents(ctx, target.Owner, target.Repository, path, &gh.RepositoryContentGetOptions{Ref: ref})
	if err != nil {
		return "", wrapRepositoryContentError(path, err)
	}
	return decodeRepositoryContent(path, file)
}

func (c *RepositoryContent) Paths(ctx context.Context, target pullrequest.Target, ref string) ([]string, error) {
	client, err := c.clients.Client(ctx, target.InstallationID)
	if err != nil {
		return nil, err
	}
	tree, _, err := client.Git.GetTree(ctx, target.Owner, target.Repository, ref, true)
	if err != nil {
		return nil, fmt.Errorf("저장소 트리를 읽지 못했습니다: %w", err)
	}
	paths := make([]string, 0, len(tree.Entries))
	for _, entry := range tree.Entries {
		if entry.GetType() != "blob" {
			continue
		}
		paths = append(paths, entry.GetPath())
	}
	return paths, nil
}
