package reviewpublication

import (
	"context"
	"fmt"
)

func (r *Reconciler) reconcileRuns(ctx context.Context) {
	now := r.deps.Clock.Now()
	if r.throughID == 0 {
		highWatermark, err := r.deps.Candidates.PublicationCandidateHighWatermark(ctx, now)
		if err != nil {
			r.deps.Logger.Error("리뷰 게시 결과 조정 범위를 읽지 못했습니다", "error", err)
			return
		}
		if highWatermark == 0 {
			r.afterID = 0
			return
		}
		r.throughID = highWatermark
	}
	runs, err := r.deps.Candidates.PublicationCandidates(ctx, now, r.afterID, r.throughID, r.config.BatchSize)
	if err != nil {
		r.deps.Logger.Error("리뷰 게시 결과 조정 대상을 읽지 못했습니다", "error", err)
		return
	}
	for _, run := range runs {
		r.afterID = run.ID
		reconcileContext, cancel := context.WithTimeout(ctx, r.config.ReconcileTimeout)
		err := r.reconcileRun(reconcileContext, run)
		cancel()
		if err != nil {
			r.deps.Logger.Warn("리뷰 게시 결과를 조정하지 못했습니다", "run", run.ID, "target", fmt.Sprintf("%s/%s#%d", run.Owner, run.Repository, run.Number), "error", err)
		}
	}
	if len(runs) < r.config.BatchSize || r.afterID >= r.throughID {
		r.afterID = 0
		r.throughID = 0
	}
}
