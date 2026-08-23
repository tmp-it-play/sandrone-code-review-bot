package reviewpullrequest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/job"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/review"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
	"github.com/it-play/sandrone-code-review-bot/internal/core/setting"
)

const reviewPromptVersion = "review-v1"

func newWorkflowRun(task job.ReviewJob, target pullrequest.Target, config setting.RepoConfig, startedAt time.Time, snapshotObservedAt time.Time) (reviewworkflow.Run, error) {
	configHash, err := hashJSON(config)
	if err != nil {
		return reviewworkflow.Run{}, fmt.Errorf("리뷰 설정 hash를 만들지 못했습니다: %w", err)
	}
	modelPolicyHash, err := hashJSON(config.Sandrone.Providers)
	if err != nil {
		return reviewworkflow.Run{}, fmt.Errorf("모델 정책 hash를 만들지 못했습니다: %w", err)
	}
	requestIdentity := workflowRequestIdentity(task)
	key := workflowHash(
		"review-run-v1",
		strconv.FormatInt(target.InstallationID, 10),
		target.Owner,
		target.Repository,
		strconv.Itoa(target.Number),
		target.BaseSHA,
		target.HeadSHA,
		configHash,
		reviewPromptVersion,
		modelPolicyHash,
		requestIdentity,
	)
	return reviewworkflow.Run{
		Key:                key,
		InstallationID:     target.InstallationID,
		Owner:              target.Owner,
		Repository:         target.Repository,
		Number:             target.Number,
		BaseSHA:            target.BaseSHA,
		HeadSHA:            target.HeadSHA,
		ConfigHash:         configHash,
		PromptVersion:      reviewPromptVersion,
		ModelPolicyHash:    modelPolicyHash,
		RequestIdentity:    requestIdentity,
		Trigger:            task.Trigger,
		Status:             reviewworkflow.RunStatusPlanning,
		SnapshotObservedAt: snapshotObservedAt,
		SnapshotOrderKey:   task.SnapshotOrderKey,
		StartedAt:          startedAt,
		HeartbeatAt:        startedAt,
	}, nil
}

func (u *UseCase) finishWorkflow(ctx context.Context, runID uint64, leaseToken string, status reviewworkflow.RunStatus, detail string, advanceWatermark bool) (reviewworkflow.RunStatus, error) {
	return u.finishWorkflowWithOutcome(ctx, runID, leaseToken, status, reviewOutcomeOf(status), detail, advanceWatermark)
}

func (u *UseCase) finishWorkflowWithOutcome(ctx context.Context, runID uint64, leaseToken string, status reviewworkflow.RunStatus, outcome review.Outcome, detail string, advanceWatermark bool) (reviewworkflow.RunStatus, error) {
	terminalAt := u.deps.Clock.Now()
	return u.deps.Workflows.FinishRun(ctx, runID, reviewworkflow.RunResult{
		Status:           status,
		ReviewOutcome:    outcome,
		Error:            detail,
		TerminalAt:       terminalAt,
		ExpiresAt:        terminalAt.Add(u.deps.Retention),
		AdvanceWatermark: advanceWatermark,
		LeaseToken:       leaseToken,
	})
}

func reviewOutcomeOf(status reviewworkflow.RunStatus) review.Outcome {
	switch status {
	case reviewworkflow.RunStatusComplete:
		return review.OutcomeSucceeded
	case reviewworkflow.RunStatusPartial:
		return review.OutcomePartial
	case reviewworkflow.RunStatusSuperseded:
		return review.OutcomeSuperseded
	case reviewworkflow.RunStatusSkipped:
		return review.OutcomeSkipped
	default:
		return review.OutcomeUnavailable
	}
}

func transitionCoverage(items []reviewworkflow.CoverageItem, unitHash string, status reviewworkflow.CoverageStatus, reviewedAt *time.Time) {
	for index := range items {
		if items[index].UnitHash != unitHash || items[index].Status != reviewworkflow.CoverageStatusPlanned {
			continue
		}
		items[index].Status = status
		items[index].ReviewedAt = reviewedAt
	}
}

func coverageDetail(summary reviewworkflow.CoverageSummary) string {
	return fmt.Sprintf(
		"coverage total=%d reviewed=%d failed=%d deferred=%d skipped=%d pending=%d",
		summary.Total,
		summary.Reviewed,
		summary.Failed,
		summary.Deferred,
		summary.Skipped,
		summary.Pending,
	)
}

func workflowRequestIdentity(task job.ReviewJob) string {
	if task.RequestIdentity != "" {
		return task.RequestIdentity
	}
	if task.Trigger.IsAutomatic() {
		return ""
	}
	if task.CommentID != 0 {
		return "comment:" + strconv.FormatInt(task.CommentID, 10)
	}
	return workflowHash("manual-review-v1", string(task.Trigger), task.Invoker, task.Instruction)
}

func hashJSON(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return workflowHash(string(encoded)), nil
}

func workflowHash(parts ...string) string {
	hash := sha256.New()
	for _, part := range parts {
		hash.Write([]byte(strconv.Itoa(len(part))))
		hash.Write([]byte{0})
		hash.Write([]byte(part))
	}
	return hex.EncodeToString(hash.Sum(nil))
}
