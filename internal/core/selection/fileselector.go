package selection

import (
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
)

type FileSelector struct {
	Include  []string
	Exclude  []string
	MaxFiles int
}

func (s FileSelector) Select(files []pullrequest.ChangedFile) Selection {
	kept := make([]pullrequest.ChangedFile, 0, len(files))
	for _, file := range files {
		if file.IsRemoved() {
			continue
		}
		if strings.TrimSpace(file.Patch) == "" {
			continue
		}
		if !s.allowed(file.Path) {
			continue
		}
		kept = append(kept, file)
	}
	sort.SliceStable(kept, func(left, right int) bool {
		return kept[left].ChangeSize() > kept[right].ChangeSize()
	})
	var skipped []pullrequest.ChangedFile
	if s.MaxFiles > 0 && len(kept) > s.MaxFiles {
		skipped = append(skipped, kept[s.MaxFiles:]...)
		kept = kept[:s.MaxFiles]
	}
	sortByPath(kept)
	sortByPath(skipped)
	return Selection{Files: kept, Skipped: skipped}
}

func sortByPath(files []pullrequest.ChangedFile) {
	sort.SliceStable(files, func(left, right int) bool {
		return files[left].Path < files[right].Path
	})
}

func (s FileSelector) allowed(path string) bool {
	if len(s.Include) > 0 && !matchesAny(s.Include, path) {
		return false
	}
	return !matchesAny(s.Exclude, path)
}

func matchesAny(patterns []string, path string) bool {
	for _, pattern := range patterns {
		if pattern == "" {
			continue
		}
		if matched, err := doublestar.Match(pattern, path); err == nil && matched {
			return true
		}
		if matched, err := doublestar.Match(strings.TrimPrefix(pattern, "**/"), path); err == nil && matched {
			return true
		}
	}
	return false
}
