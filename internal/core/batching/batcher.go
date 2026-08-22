package batching

import "github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"

const perFileOverhead = 220

type Batcher struct {
	MaxChars   int
	MaxBatches int
}

func (b Batcher) Split(files []pullrequest.ChangedFile, reserved int) Plan {
	if len(files) == 0 {
		return Plan{}
	}
	budget := b.MaxChars - reserved
	if budget <= 0 {
		budget = b.MaxChars
	}
	if budget <= 0 {
		return Plan{Batches: [][]pullrequest.ChangedFile{files}}
	}

	plan := Plan{}
	current := make([]pullrequest.ChangedFile, 0, len(files))
	used := 0
	for _, file := range files {
		cost := fileCost(file)
		if len(current) > 0 && used+cost > budget {
			plan.Batches = append(plan.Batches, current)
			current = make([]pullrequest.ChangedFile, 0, len(files))
			used = 0
		}
		current = append(current, file)
		used += cost
	}
	if len(current) > 0 {
		plan.Batches = append(plan.Batches, current)
	}

	if b.MaxBatches > 0 && len(plan.Batches) > b.MaxBatches {
		for _, batch := range plan.Batches[b.MaxBatches:] {
			plan.Overflow = append(plan.Overflow, batch...)
		}
		plan.Batches = plan.Batches[:b.MaxBatches]
	}
	return plan
}

func fileCost(file pullrequest.ChangedFile) int {
	return len(file.Patch) + len(file.Content) + len(file.Path) + perFileOverhead
}
