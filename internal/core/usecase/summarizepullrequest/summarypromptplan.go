package summarizepullrequest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/it-play/sandrone-code-review-bot/internal/core/instruction"
	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
	"github.com/it-play/sandrone-code-review-bot/internal/core/prompt"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"github.com/it-play/sandrone-code-review-bot/internal/core/selection"
	"github.com/it-play/sandrone-code-review-bot/internal/core/setting"
)

type summaryPromptPlan struct {
	Messages     []llm.Message
	Unreviewed   []review.UnreviewedFile
	Coverage     review.CoverageView
	ResultPolicy string
}

func (u *UseCase) buildSummaryPromptPlan(request pullrequest.PullRequest, selected []pullrequest.ChangedFile, chosen selection.Selection, instructions instruction.Collection, config setting.RepoConfig, extra string, completionRequest llm.Request) (summaryPromptPlan, error) {
	budget := u.deps.Completer.PromptBudgetFor(completionRequest)
	if budget <= 0 {
		return summaryPromptPlan{}, fmt.Errorf("요약에 사용할 수 있는 LLM 입력 한도가 없습니다")
	}
	if config.MaxPromptChars > 0 && config.MaxPromptChars < budget {
		budget = config.MaxPromptChars
	}
	extra, err := u.fitSummaryExtra(request, config, extra, budget)
	if err != nil {
		return summaryPromptPlan{}, err
	}
	included := make([]pullrequest.ChangedFile, 0, len(selected))
	states := map[string]string{}
	for _, file := range selected {
		candidate := append(append([]pullrequest.ChangedFile{}, included...), file)
		if u.summaryMessagesFit(request, candidate, instruction.Collection{}, config, extra, budget) {
			included = candidate
			if file.PatchTruncated {
				states[file.Path] = "파일별 입력 한도로 diff 일부 생략"
			} else {
				states[file.Path] = "reviewed"
			}
			continue
		}
		partial, fits := u.fitSummaryFile(request, included, file, config, extra, budget)
		if fits {
			included = append(included, partial)
			states[file.Path] = "전체 입력 한도로 diff 일부 생략"
		} else {
			states[file.Path] = "전체 입력 한도로 파일 diff 생략"
		}
		break
	}
	for _, file := range selected {
		if _, exists := states[file.Path]; !exists {
			states[file.Path] = "전체 입력 한도로 파일 diff 생략"
		}
	}
	fittedInstructions := u.fitSummaryInstructions(request, included, instructions, config, extra, budget)
	messages := u.summaryMessages(request, included, fittedInstructions, config, extra)
	if llm.MessagesSize(messages) > budget {
		return summaryPromptPlan{}, fmt.Errorf("요약 프롬프트가 입력 한도 %d자를 넘었습니다", budget)
	}
	plan := summaryPromptPlan{Messages: messages}
	plan.Coverage.Total = len(selected) + len(chosen.Skipped) + len(chosen.Excluded)
	for _, file := range selected {
		reason := states[file.Path]
		if reason == "reviewed" {
			plan.Coverage.Reviewed++
			continue
		}
		plan.Coverage.Deferred++
		plan.Unreviewed = append(plan.Unreviewed, summaryUnreviewed(file, reason))
	}
	for _, file := range chosen.Skipped {
		plan.Coverage.Deferred++
		plan.Unreviewed = append(plan.Unreviewed, summaryUnreviewed(file, "파일 수 한도로 생략"))
	}
	for _, excluded := range chosen.Excluded {
		plan.Coverage.Skipped++
		plan.Unreviewed = append(plan.Unreviewed, summaryUnreviewed(excluded.File, string(excluded.Reason)))
	}
	plan.Coverage.Status = "complete"
	if plan.Coverage.Deferred > 0 || plan.Coverage.Failed > 0 || plan.Coverage.Pending > 0 {
		plan.Coverage.Status = "partial"
	}
	sort.SliceStable(plan.Unreviewed, func(left int, right int) bool {
		if plan.Unreviewed[left].Path != plan.Unreviewed[right].Path {
			return plan.Unreviewed[left].Path < plan.Unreviewed[right].Path
		}
		return plan.Unreviewed[left].Reason < plan.Unreviewed[right].Reason
	})
	for index := range plan.Unreviewed {
		plan.Unreviewed[index].Path = u.deps.Masker.Mask(plan.Unreviewed[index].Path)
	}
	policy, err := summaryResultPolicy(plan.Coverage, plan.Unreviewed, budget)
	if err != nil {
		return summaryPromptPlan{}, err
	}
	plan.ResultPolicy = policy
	return plan, nil
}

func (u *UseCase) summaryMessages(request pullrequest.PullRequest, files []pullrequest.ChangedFile, instructions instruction.Collection, config setting.RepoConfig, extra string) []llm.Message {
	messages := prompt.SummaryPrompt{
		Context: prompt.Context{
			PullRequest:  request,
			Files:        files,
			Instructions: instructions,
			Config:       config,
		},
		Extra: extra,
	}.Messages()
	return llm.MaskMessages(messages, u.deps.Masker.Mask)
}

func (u *UseCase) summaryMessagesFit(request pullrequest.PullRequest, files []pullrequest.ChangedFile, instructions instruction.Collection, config setting.RepoConfig, extra string, budget int) bool {
	return llm.MessagesSize(u.summaryMessages(request, files, instructions, config, extra)) <= budget
}

func (u *UseCase) fitSummaryExtra(request pullrequest.PullRequest, config setting.RepoConfig, extra string, budget int) (string, error) {
	if u.summaryMessagesFit(request, nil, instruction.Collection{}, config, extra, budget) {
		return extra, nil
	}
	if !u.summaryMessagesFit(request, nil, instruction.Collection{}, config, "", budget) {
		return "", fmt.Errorf("PR 기본 정보가 요약 입력 한도 %d자를 넘었습니다", budget)
	}
	low := 0
	high := len(extra)
	best := ""
	for low <= high {
		middle := low + (high-low)/2
		candidate := truncateUTF8(extra, middle)
		if candidate != "" {
			candidate += "\n[추가 요청 일부 생략]"
		}
		if u.summaryMessagesFit(request, nil, instruction.Collection{}, config, candidate, budget) {
			best = candidate
			low = middle + 1
		} else {
			high = middle - 1
		}
	}
	return best, nil
}

func (u *UseCase) fitSummaryFile(request pullrequest.PullRequest, included []pullrequest.ChangedFile, file pullrequest.ChangedFile, config setting.RepoConfig, extra string, budget int) (pullrequest.ChangedFile, bool) {
	const notice = "\n[전체 입력 한도로 diff 이후 내용 생략]"
	low := 0
	high := len(file.Patch)
	best := pullrequest.ChangedFile{}
	found := false
	for low <= high {
		middle := low + (high-low)/2
		candidate := file
		candidate.Patch = truncateUTF8(file.Patch, middle)
		if candidate.Patch != "" {
			candidate.Patch += notice
		}
		candidate.PatchTruncated = true
		files := append(append([]pullrequest.ChangedFile{}, included...), candidate)
		if u.summaryMessagesFit(request, files, instruction.Collection{}, config, extra, budget) {
			best = candidate
			found = candidate.Patch != ""
			low = middle + 1
		} else {
			high = middle - 1
		}
	}
	return best, found
}

func (u *UseCase) fitSummaryInstructions(request pullrequest.PullRequest, files []pullrequest.ChangedFile, source instruction.Collection, config setting.RepoConfig, extra string, budget int) instruction.Collection {
	if u.summaryMessagesFit(request, files, source, config, extra, budget) {
		return source
	}
	result := instruction.Collection{}
	omitted := append([]string{}, source.Omitted...)
	for index, document := range source.Documents {
		candidate := result
		candidate.Documents = append(append([]instruction.Document{}, result.Documents...), document)
		if u.summaryMessagesFit(request, files, candidate, config, extra, budget) {
			result = candidate
			continue
		}
		low := 0
		high := len(document.Content)
		best := instruction.Document{}
		found := false
		for low <= high {
			middle := low + (high-low)/2
			partial := document
			partial.Content = truncateUTF8(document.Content, middle)
			partial.Truncated = true
			candidate = result
			candidate.Documents = append(append([]instruction.Document{}, result.Documents...), partial)
			if partial.Content != "" && u.summaryMessagesFit(request, files, candidate, config, extra, budget) {
				best = partial
				found = true
				low = middle + 1
			} else {
				high = middle - 1
			}
		}
		if found {
			result.Documents = append(result.Documents, best)
		}
		for _, remaining := range source.Documents[index:] {
			omitted = append(omitted, remaining.Path)
		}
		break
	}
	for _, path := range omitted {
		candidate := result
		candidate.Omitted = append(append([]string{}, result.Omitted...), path)
		if !u.summaryMessagesFit(request, files, candidate, config, extra, budget) {
			break
		}
		result = candidate
	}
	return result
}

func summaryUnreviewed(file pullrequest.ChangedFile, reason string) review.UnreviewedFile {
	return review.UnreviewedFile{Path: file.Path, Additions: file.Additions, Deletions: file.Deletions, Reason: reason}
}

func summaryResultPolicy(coverage review.CoverageView, unreviewed []review.UnreviewedFile, budget int) (string, error) {
	encoded, err := json.Marshal(struct {
		Version    string
		Budget     int
		Coverage   review.CoverageView
		Unreviewed []review.UnreviewedFile
	}{Version: "summary-result-v2", Budget: budget, Coverage: coverage, Unreviewed: unreviewed})
	if err != nil {
		return "", fmt.Errorf("요약 범위 식별자를 만들지 못했습니다: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return "summary-canonical-v2:" + hex.EncodeToString(digest[:]), nil
}
