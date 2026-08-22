package github

import (
	"context"
	"fmt"

	gh "github.com/google/go-github/v90/github"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
)

type PullRequestSource struct {
	clients *ClientFactory
}

func NewPullRequestSource(clients *ClientFactory) *PullRequestSource {
	return &PullRequestSource{clients: clients}
}

func (s *PullRequestSource) PullRequest(ctx context.Context, target pullrequest.Target) (pullrequest.PullRequest, error) {
	client, err := s.clients.Client(ctx, target.InstallationID)
	if err != nil {
		return pullrequest.PullRequest{}, err
	}
	found, _, err := client.PullRequests.Get(ctx, target.Owner, target.Repository, target.Number)
	if err != nil {
		return pullrequest.PullRequest{}, fmt.Errorf("PR %s를 읽지 못했다: %w", target.Reference(), err)
	}
	return pullrequest.PullRequest{
		Number:  found.GetNumber(),
		Title:   found.GetTitle(),
		Body:    found.GetBody(),
		Author:  found.GetUser().GetLogin(),
		BaseRef: found.GetBase().GetRef(),
		HeadRef: found.GetHead().GetRef(),
		BaseSHA: found.GetBase().GetSHA(),
		HeadSHA: found.GetHead().GetSHA(),
		State:   found.GetState(),
		Draft:   found.GetDraft(),
	}, nil
}

func (s *PullRequestSource) ChangedFiles(ctx context.Context, target pullrequest.Target) ([]pullrequest.ChangedFile, error) {
	client, err := s.clients.Client(ctx, target.InstallationID)
	if err != nil {
		return nil, err
	}
	options := &gh.ListOptions{PerPage: 100}
	collected := make([]pullrequest.ChangedFile, 0, 64)
	for {
		files, response, listErr := client.PullRequests.ListFiles(ctx, target.Owner, target.Repository, target.Number, options)
		if listErr != nil {
			return nil, fmt.Errorf("PR 변경 파일을 읽지 못했다: %w", listErr)
		}
		for _, file := range files {
			collected = append(collected, toChangedFile(file))
		}
		if response == nil || response.NextPage == 0 {
			break
		}
		options.Page = response.NextPage
	}
	return collected, nil
}

func (s *PullRequestSource) ChangedFilesBetween(ctx context.Context, target pullrequest.Target, baseSHA string, headSHA string) ([]pullrequest.ChangedFile, error) {
	client, err := s.clients.Client(ctx, target.InstallationID)
	if err != nil {
		return nil, err
	}
	options := &gh.ListOptions{PerPage: 100}
	collected := make([]pullrequest.ChangedFile, 0, 64)
	for {
		comparison, response, compareErr := client.Repositories.CompareCommits(ctx, target.Owner, target.Repository, baseSHA, headSHA, options)
		if compareErr != nil {
			return nil, fmt.Errorf("커밋 비교에 실패했다: %w", compareErr)
		}
		for _, file := range comparison.Files {
			collected = append(collected, toChangedFile(file))
		}
		if response == nil || response.NextPage == 0 {
			break
		}
		options.Page = response.NextPage
	}
	return collected, nil
}

func (s *PullRequestSource) FileContent(ctx context.Context, target pullrequest.Target, path string, ref string) (string, error) {
	client, err := s.clients.Client(ctx, target.InstallationID)
	if err != nil {
		return "", err
	}
	file, _, _, err := client.Repositories.GetContents(ctx, target.Owner, target.Repository, path, &gh.RepositoryContentGetOptions{Ref: ref})
	if err != nil {
		return "", fmt.Errorf("%s 파일을 읽지 못했다: %w", path, err)
	}
	if file == nil {
		return "", fmt.Errorf("%s는 파일이 아니다", path)
	}
	return file.GetContent()
}

func toChangedFile(file *gh.CommitFile) pullrequest.ChangedFile {
	return pullrequest.ChangedFile{
		Path:         file.GetFilename(),
		PreviousPath: file.GetPreviousFilename(),
		Status:       file.GetStatus(),
		Additions:    file.GetAdditions(),
		Deletions:    file.GetDeletions(),
		Patch:        file.GetPatch(),
	}
}
