package pullrequest

import (
	"strings"
	"unicode/utf8"
)

type ChangedFile struct {
	Path           string
	PreviousPath   string
	Status         string
	Additions      int
	Deletions      int
	Patch          string
	Content        string
	Truncated      bool
	PatchTruncated bool
}

func (f ChangedFile) IsRemoved() bool {
	return f.Status == "removed"
}

func (f ChangedFile) ChangeSize() int {
	return f.Additions + f.Deletions
}

func (f ChangedFile) WithSourceLimit(limit int) ChangedFile {
	if len(f.Content) <= limit {
		return f
	}
	limit = max(0, limit)
	for limit > 0 && !utf8.RuneStart(f.Content[limit]) {
		limit--
	}
	prefix := f.Content[:limit]
	if end := strings.LastIndexByte(prefix, '\n'); end >= 0 {
		prefix = prefix[:end+1]
	}
	f.Content = prefix
	f.Truncated = true
	return f
}

func (f ChangedFile) SourceScope() string {
	if f.Content == "" {
		return "unavailable"
	}
	if f.Truncated {
		return "prefix"
	}
	return "complete"
}
