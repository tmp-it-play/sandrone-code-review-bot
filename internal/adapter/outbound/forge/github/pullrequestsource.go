package github

import (
	"context"
	"fmt"
	"strings"

	gh "github.com/google/go-github/v90/github"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
)

type PullRequestSource struct {
	clients *ClientFactory
}

const compareFilesSafetyLimit = 300

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
		return pullrequest.PullRequest{}, fmt.Errorf("PR %s를 읽지 못했습니다: %w", target.Reference(), err)
	}
	repository := found.GetBase().GetRepo()
	visibility := strings.TrimSpace(repository.GetVisibility())
	return pullrequest.PullRequest{
		Number:       found.GetNumber(),
		Title:        found.GetTitle(),
		Body:         found.GetBody(),
		Author:       found.GetUser().GetLogin(),
		BaseRef:      found.GetBase().GetRef(),
		HeadRef:      found.GetHead().GetRef(),
		BaseSHA:      found.GetBase().GetSHA(),
		HeadSHA:      found.GetHead().GetSHA(),
		State:        found.GetState(),
		Draft:        found.GetDraft(),
		Private:      repository.GetPrivate() || !strings.EqualFold(visibility, "public"),
		ChangedFiles: found.GetChangedFiles(),
		UpdatedAt:    found.GetUpdatedAt().Time,
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
			return nil, fmt.Errorf("PR 변경 파일을 읽지 못했습니다: %w", listErr)
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
	return changedFilesBetween(ctx, client, target, baseSHA, headSHA)
}

func changedFilesBetween(ctx context.Context, client *gh.Client, target pullrequest.Target, baseSHA string, headSHA string) ([]pullrequest.ChangedFile, error) {
	comparison, _, err := client.Repositories.CompareCommits(ctx, target.Owner, target.Repository, baseSHA, headSHA, &gh.ListOptions{PerPage: 100})
	if err != nil {
		return nil, fmt.Errorf("커밋 비교에 실패했습니다: %w", err)
	}
	if len(comparison.Files) >= compareFilesSafetyLimit {
		return nil, fmt.Errorf("커밋 비교 파일이 API 안전 상한 %d개에 도달했습니다", compareFilesSafetyLimit)
	}
	collected := make([]pullrequest.ChangedFile, 0, len(comparison.Files))
	for _, file := range comparison.Files {
		collected = append(collected, toChangedFile(file))
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
		return "", wrapRepositoryContentError(path, err)
	}
	return decodeRepositoryContent(path, file)
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
