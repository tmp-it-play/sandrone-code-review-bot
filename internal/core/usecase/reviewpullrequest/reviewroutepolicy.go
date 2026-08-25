package reviewpullrequest

import (
	"sort"

	"github.com/it-play/sandrone-code-review-bot/internal/core/diff"
	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
)

type reviewRoutePolicy struct {
	promptLimit     int
	outputLimit     int
	targetOutput    int
	maximumFindings int
	outputRoutes    []llm.RouteCapacity
}

const reviewOutputPromptRatio = 16

const minimumReviewOutputTokens = 2048

func reviewOutputTokens(configured int) int {
	if configured < minimumReviewOutputTokens {
		return minimumReviewOutputTokens
	}
	return configured
}

func newReviewRoutePolicy(capacities []llm.RouteCapacity, configuredOutput int) reviewRoutePolicy {
	promptLimits := make([]int, 0, len(capacities))
	outputLimits := make([]int, 0, len(capacities))
	outputRoutes := make([]llm.RouteCapacity, 0, len(capacities))
	for _, capacity := range capacities {
		if capacity.PromptChars > 0 {
			promptLimits = append(promptLimits, capacity.PromptChars)
		}
		nominalOutput := capacity.MaxOutputTokens
		if configuredOutput > 0 && (nominalOutput <= 0 || nominalOutput > configuredOutput) {
			nominalOutput = configuredOutput
		}
		output := capacity.UsableOutputTokens
		if output <= 0 {
			output = nominalOutput
		}
		if nominalOutput <= 0 || output <= 0 {
			continue
		}
		if output > nominalOutput {
			output = nominalOutput
		}
		outputLimits = append(outputLimits, output)
		capacity.MaxOutputTokens = nominalOutput
		capacity.UsableOutputTokens = output
		outputRoutes = append(outputRoutes, capacity)
	}
	promptLimit := resilientCapacity(promptLimits)
	outputLimit := resilientCapacity(outputLimits)
	if outputLimit <= 0 {
		outputLimit = configuredOutput
	}
	if outputLimit <= 0 {
		outputLimit = 8192
	}
	targetOutput := outputLimit * 3 / 4
	if targetOutput < 1200 {
		targetOutput = min(outputLimit, 1200)
	}
	outputDrivenPromptLimit := targetOutput * reviewOutputPromptRatio
	if promptLimit <= 0 || outputDrivenPromptLimit < promptLimit {
		promptLimit = outputDrivenPromptLimit
	}
	maximumFindings := (targetOutput - 700) / 600
	if maximumFindings < 1 {
		maximumFindings = 1
	}
	if maximumFindings > 8 {
		maximumFindings = 8
	}
	return reviewRoutePolicy{
		promptLimit:     promptLimit,
		outputLimit:     outputLimit,
		targetOutput:    targetOutput,
		maximumFindings: maximumFindings,
		outputRoutes:    outputRoutes,
	}
}

func (p reviewRoutePolicy) BatchPolicy(files []pullrequest.ChangedFile, includeFileNotes bool) reviewBatchPolicy {
	required := estimatedReviewOutputTokens(files, includeFileNotes)
	if required < 1024 {
		required = 1024
	}
	if required > p.targetOutput {
		required = p.targetOutput
	}
	usableMaximum := required * 3 / 2
	if usableMaximum < 2048 {
		usableMaximum = 2048
	}
	if usableMaximum > p.outputLimit {
		usableMaximum = p.outputLimit
	}
	if usableMaximum < required {
		usableMaximum = required
	}
	maximumFindings := (usableMaximum - 700) / 600
	if maximumFindings < 1 {
		maximumFindings = 1
	}
	if maximumFindings > p.maximumFindings {
		maximumFindings = p.maximumFindings
	}
	return reviewBatchPolicy{MaxOutputTokens: p.nominalOutputTokens(usableMaximum), RequiredOutputTokens: required, MaxFindings: maximumFindings}
}

func (p reviewRoutePolicy) nominalOutputTokens(usable int) int {
	if usable <= 0 || len(p.outputRoutes) == 0 {
		return usable
	}
	required := make([]int, 0, len(p.outputRoutes))
	for _, route := range p.outputRoutes {
		if route.UsableOutputTokens < usable {
			continue
		}
		nominal := (usable*route.MaxOutputTokens + route.UsableOutputTokens - 1) / route.UsableOutputTokens
		if nominal < usable {
			nominal = usable
		}
		if nominal <= route.MaxOutputTokens {
			required = append(required, nominal)
		}
	}
	quorum := routeQuorum(len(p.outputRoutes))
	if len(required) < quorum {
		return usable
	}
	sort.Ints(required)
	return required[quorum-1]
}

func (p reviewRoutePolicy) ShouldSplit(files []pullrequest.ChangedFile, promptSize int, includeFileNotes bool) bool {
	if len(files) < 2 {
		return false
	}
	return p.promptLimit > 0 && promptSize > p.promptLimit || estimatedReviewOutputTokens(files, includeFileNotes) > p.targetOutput
}

func (p reviewRoutePolicy) Split(files []pullrequest.ChangedFile) ([]pullrequest.ChangedFile, []pullrequest.ChangedFile) {
	if len(files) < 2 {
		return append([]pullrequest.ChangedFile(nil), files...), nil
	}
	total := 0
	for _, file := range files {
		total += reviewFileWeight(file)
	}
	leftWeight := 0
	splitAt := 1
	for index := 0; index < len(files)-1; index++ {
		leftWeight += reviewFileWeight(files[index])
		splitAt = index + 1
		if leftWeight*2 >= total {
			break
		}
	}
	return append([]pullrequest.ChangedFile(nil), files[:splitAt]...), append([]pullrequest.ChangedFile(nil), files[splitAt:]...)
}

func resilientCapacity(values []int) int {
	if len(values) == 0 {
		return 0
	}
	sort.Sort(sort.Reverse(sort.IntSlice(values)))
	quorum := routeQuorum(len(values))
	return values[quorum-1]
}

func routeQuorum(routes int) int {
	quorum := (routes*2 + 2) / 3
	if quorum < 1 {
		return 1
	}
	return quorum
}

func estimatedReviewOutputTokens(files []pullrequest.ChangedFile, includeFileNotes bool) int {
	hunks := 0
	changes := 0
	for _, file := range files {
		hunks += len((diff.Parser{}).Parse(file.Patch))
		changes += file.ChangeSize()
	}
	findings := (hunks + 3) / 4
	if findings < 1 && len(files) > 0 {
		findings = 1
	}
	if findings > 8 {
		findings = 8
	}
	fileTokens := len(files) * 70
	if includeFileNotes {
		fileTokens = len(files) * 150
	}
	changeTokens := changes / 4
	if changeTokens > 1200 {
		changeTokens = 1200
	}
	return 700 + fileTokens + findings*600 + changeTokens
}

func reviewFileWeight(file pullrequest.ChangedFile) int {
	hunks := len((diff.Parser{}).Parse(file.Patch))
	return len(file.Patch) + len(file.Content)/2 + 600 + hunks*1200 + min(file.ChangeSize(), 1200)*4
}
