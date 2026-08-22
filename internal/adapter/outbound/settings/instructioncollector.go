package settings

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/it-play/sandrone-code-review-bot/internal/core/instruction"
	"github.com/it-play/sandrone-code-review-bot/internal/core/port/outbound"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/setting"
)

type InstructionCollector struct {
	content outbound.RepositoryContent
	logger  *slog.Logger
}

func NewInstructionCollector(content outbound.RepositoryContent, logger *slog.Logger) *InstructionCollector {
	return &InstructionCollector{content: content, logger: logger}
}

func (c *InstructionCollector) Instructions(ctx context.Context, target pullrequest.Target, config setting.RepoConfig) (instruction.Collection, error) {
	patterns := config.Sandrone.InstructionFiles
	if len(patterns) == 0 {
		patterns = setting.DefaultInstructionFiles()
	}
	paths := c.resolve(ctx, target, patterns)
	budget := instruction.NewBudget(config.Sandrone.MaxInstructionChars)
	collection := instruction.Collection{}
	seen := map[string]struct{}{}
	for _, path := range paths {
		if budget.Exhausted() {
			collection.Omitted = append(collection.Omitted, path)
			continue
		}
		body, err := c.readFirst(ctx, target, path)
		if err != nil {
			continue
		}
		digest := fingerprint(body)
		if _, duplicate := seen[digest]; duplicate {
			continue
		}
		seen[digest] = struct{}{}
		taken, truncated := budget.Take(body)
		if strings.TrimSpace(taken) == "" {
			collection.Omitted = append(collection.Omitted, path)
			continue
		}
		collection.Documents = append(collection.Documents, instruction.Document{
			Path:      path,
			Content:   taken,
			Truncated: truncated,
		})
	}
	return collection, nil
}

func (c *InstructionCollector) resolve(ctx context.Context, target pullrequest.Target, patterns []string) []string {
	var tree []string
	treeLoaded := false
	resolved := make([]string, 0, len(patterns))
	seen := map[string]struct{}{}
	for _, pattern := range patterns {
		if !strings.ContainsAny(pattern, "*?[") {
			if _, ok := seen[pattern]; !ok {
				seen[pattern] = struct{}{}
				resolved = append(resolved, pattern)
			}
			continue
		}
		if !treeLoaded {
			tree = c.listFirst(ctx, target)
			treeLoaded = true
		}
		matched := make([]string, 0, 8)
		for _, path := range tree {
			if ok, err := doublestar.Match(pattern, path); err == nil && ok {
				matched = append(matched, path)
			}
		}
		sort.Strings(matched)
		for _, path := range matched {
			if _, ok := seen[path]; ok {
				continue
			}
			seen[path] = struct{}{}
			resolved = append(resolved, path)
		}
	}
	return resolved
}

func fingerprint(body string) string {
	normalized := strings.Join(strings.Fields(body), " ")
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

func (c *InstructionCollector) readFirst(ctx context.Context, target pullrequest.Target, path string) (string, error) {
	var lastErr error
	for _, ref := range target.ContentRefs() {
		body, err := c.content.File(ctx, target, path, ref)
		if err == nil {
			return body, nil
		}
		lastErr = err
	}
	return "", lastErr
}

func (c *InstructionCollector) listFirst(ctx context.Context, target pullrequest.Target) []string {
	for _, ref := range target.ContentRefs() {
		paths, err := c.content.Paths(ctx, target, ref)
		if err == nil {
			return paths
		}
		c.logger.Warn("저장소 파일 목록을 읽지 못했습니다", "target", target.FullName(), "ref", refLabel(ref), "error", err)
	}
	return nil
}
