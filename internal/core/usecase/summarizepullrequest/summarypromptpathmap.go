package summarizepullrequest

import (
	"fmt"
	"sort"
	"strings"

	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

type summaryPromptPathMap struct {
	rawToPrompt      map[string]string
	promptToRaw      map[string]string
	replacementPaths []string
	promptAliases    []string
}

func newSummaryPromptPathMap(files []pullrequest.ChangedFile, mask func(string) string) *summaryPromptPathMap {
	mapping := &summaryPromptPathMap{rawToPrompt: map[string]string{}, promptToRaw: map[string]string{}}
	rawPaths := map[string]struct{}{}
	for _, file := range files {
		if file.Path != "" {
			rawPaths[file.Path] = struct{}{}
		}
		if file.PreviousPath != "" {
			rawPaths[file.PreviousPath] = struct{}{}
		}
	}
	unsafePaths := make([]string, 0)
	for path := range rawPaths {
		if mask != nil && mask(path) != path {
			unsafePaths = append(unsafePaths, path)
		}
	}
	sort.Strings(unsafePaths)
	aliasOrdinal := 1
	for _, path := range unsafePaths {
		alias := ""
		for alias == "" {
			candidate := fmt.Sprintf("__sandrone_private_summary_path_%04d__", aliasOrdinal)
			aliasOrdinal++
			if _, collides := rawPaths[candidate]; collides {
				continue
			}
			if _, collides := mapping.promptToRaw[candidate]; collides {
				continue
			}
			alias = candidate
		}
		mapping.rawToPrompt[path] = alias
		mapping.promptToRaw[alias] = path
	}
	mapping.replacementPaths = append(mapping.replacementPaths, unsafePaths...)
	sort.Slice(mapping.replacementPaths, func(left int, right int) bool {
		if len(mapping.replacementPaths[left]) != len(mapping.replacementPaths[right]) {
			return len(mapping.replacementPaths[left]) > len(mapping.replacementPaths[right])
		}
		return mapping.replacementPaths[left] < mapping.replacementPaths[right]
	})
	for alias := range mapping.promptToRaw {
		mapping.promptAliases = append(mapping.promptAliases, alias)
	}
	sort.Strings(mapping.promptAliases)
	return mapping
}

func (m *summaryPromptPathMap) PromptPath(path string) string {
	if m == nil {
		return path
	}
	if promptPath, exists := m.rawToPrompt[path]; exists {
		return promptPath
	}
	return path
}

func (m *summaryPromptPathMap) RawPath(path string) string {
	if m == nil {
		return path
	}
	if rawPath, exists := m.promptToRaw[path]; exists {
		return rawPath
	}
	return path
}

func (m *summaryPromptPathMap) PromptFiles(files []pullrequest.ChangedFile, mask func(string) string) []pullrequest.ChangedFile {
	promptFiles := append([]pullrequest.ChangedFile(nil), files...)
	for index := range promptFiles {
		promptFiles[index].Path = m.PromptPath(promptFiles[index].Path)
		promptFiles[index].PreviousPath = m.PromptPath(promptFiles[index].PreviousPath)
		promptFiles[index].Status = m.Sanitize(promptFiles[index].Status, mask)
		promptFiles[index].Patch = m.Sanitize(promptFiles[index].Patch, mask)
		promptFiles[index].Content = m.Sanitize(promptFiles[index].Content, mask)
	}
	return promptFiles
}

func (m *summaryPromptPathMap) RestoreSummary(summary review.Summary) review.Summary {
	summary.Overview = m.RestoreText(summary.Overview)
	for index := range summary.Files {
		summary.Files[index].Path = m.RawPath(summary.Files[index].Path)
		summary.Files[index].Note = m.RestoreText(summary.Files[index].Note)
	}
	return summary
}

func (m *summaryPromptPathMap) RestoreText(text string) string {
	if m == nil {
		return text
	}
	for _, alias := range m.promptAliases {
		text = strings.ReplaceAll(text, alias, m.promptToRaw[alias])
	}
	return text
}

func (m *summaryPromptPathMap) Sanitize(text string, mask func(string) string) string {
	if m != nil {
		for _, rawPath := range m.replacementPaths {
			text = strings.ReplaceAll(text, rawPath, m.rawToPrompt[rawPath])
		}
	}
	if mask != nil {
		text = mask(text)
	}
	return text
}
