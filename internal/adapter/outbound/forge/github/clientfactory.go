package github

import (
	"context"
	"time"

	gh "github.com/google/go-github/v90/github"
)

const githubRequestTimeout = 90 * time.Second

type ClientFactory struct {
	tokens *TokenSource
}

func NewClientFactory(tokens *TokenSource) *ClientFactory {
	return &ClientFactory{tokens: tokens}
}

func (f *ClientFactory) Client(ctx context.Context, installationID int64) (*gh.Client, error) {
	token, err := f.tokens.InstallationToken(ctx, installationID)
	if err != nil {
		return nil, err
	}
	return gh.NewClient(gh.WithTimeout(githubRequestTimeout), gh.WithAuthToken(token))
}
