package github

import (
	"context"
	"fmt"

	"github.com/it-play/sandrone-code-review-bot/internal/core/access"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
)

type PermissionChecker struct {
	clients *ClientFactory
}

func NewPermissionChecker(clients *ClientFactory) *PermissionChecker {
	return &PermissionChecker{clients: clients}
}

func (c *PermissionChecker) Permission(ctx context.Context, target pullrequest.Target, username string) (access.Permission, error) {
	client, err := c.clients.Client(ctx, target.InstallationID)
	if err != nil {
		return access.PermissionNone, err
	}
	level, response, err := client.Repositories.GetPermissionLevel(ctx, target.Owner, target.Repository, username)
	if err != nil {
		if response != nil && response.StatusCode == 404 {
			return access.PermissionNone, nil
		}
		return access.PermissionNone, fmt.Errorf("권한을 조회하지 못했습니다: %w", err)
	}
	return access.ParsePermission(level.GetPermission()), nil
}
