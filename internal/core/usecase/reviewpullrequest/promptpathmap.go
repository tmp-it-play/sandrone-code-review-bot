package reviewpullrequest

import (
	"fmt"
	"sort"
	"strings"

	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
)

type promptPathMap struct {
	rawToPrompt map[string]string
	promptToRaw map[string]string
}

func newPromptPathMap(files []pullrequest.ChangedFile, mask func(string) string) *promptPathMap {
	mapping := &promptPathMap{rawToPrompt: map[string]string{}, promptToRaw: map[string]string{}}
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
			candidate := fmt.Sprintf("__sandrone_private_path_%04d__", aliasOrdinal)
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
	return mapping
}

func (m *promptPathMap) PromptPath(path string) string {
	if m == nil {
		return path
	}
	if promptPath, exists := m.rawToPrompt[path]; exists {
		return promptPath
	}
	return path
}

func (m *promptPathMap) RawPath(path string) string {
	if m == nil {
		return path
	}
	if rawPath, exists := m.promptToRaw[path]; exists {
		return rawPath
	}
	return path
}

func (m *promptPathMap) RestoreResult(result review.Result) review.Result {
	for index := range result.Summary.Files {
		result.Summary.Files[index].Path = m.RawPath(result.Summary.Files[index].Path)
	}
	for index := range result.Findings {
		result.Findings[index].File = m.RawPath(result.Findings[index].File)
		for occurrenceIndex := range result.Findings[index].Occurrences {
			result.Findings[index].Occurrences[occurrenceIndex].File = m.RawPath(result.Findings[index].Occurrences[occurrenceIndex].File)
		}
	}
	return result
}

func (m *promptPathMap) PromptFindings(findings []review.Finding) []review.Finding {
	promptFindings := append([]review.Finding(nil), findings...)
	for index := range promptFindings {
		promptFindings[index].File = m.PromptPath(promptFindings[index].File)
		promptFindings[index].Occurrences = append([]review.Occurrence(nil), promptFindings[index].Occurrences...)
		for occurrenceIndex := range promptFindings[index].Occurrences {
			promptFindings[index].Occurrences[occurrenceIndex].File = m.PromptPath(promptFindings[index].Occurrences[occurrenceIndex].File)
		}
	}
	return promptFindings
}

func (m *promptPathMap) PromptFiles(files []pullrequest.ChangedFile) []pullrequest.ChangedFile {
	promptFiles := append([]pullrequest.ChangedFile(nil), files...)
	for index := range promptFiles {
		promptFiles[index].Path = m.PromptPath(promptFiles[index].Path)
		promptFiles[index].PreviousPath = m.PromptPath(promptFiles[index].PreviousPath)
	}
	return promptFiles
}

func (m *promptPathMap) Sanitize(text string, mask func(string) string) string {
	for rawPath, promptPath := range m.rawToPrompt {
		text = strings.ReplaceAll(text, rawPath, promptPath)
	}
	if mask != nil {
		text = mask(text)
	}
	return text
}
