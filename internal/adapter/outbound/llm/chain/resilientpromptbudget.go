package chain

import (
	"sort"

	"github.com/it-play/sandrone-code-review-bot/internal/core/llm"
)

func resilientPromptBudget(capacities []llm.RouteCapacity) int {
	limits := make([]int, 0, len(capacities))
	for _, capacity := range capacities {
		if capacity.PromptChars > 0 {
			limits = append(limits, capacity.PromptChars)
		}
	}
	if len(limits) == 0 {
		return 0
	}
	sort.Sort(sort.Reverse(sort.IntSlice(limits)))
	quorum := (len(limits)*2 + 2) / 3
	if quorum < 1 {
		quorum = 1
	}
	return limits[quorum-1]
}
