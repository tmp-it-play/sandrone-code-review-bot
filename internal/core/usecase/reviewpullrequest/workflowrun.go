package reviewpullrequest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/job"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
	"github.com/it-play/sandrone-code-review-bot/internal/core/setting"
)

const reviewPromptVersion = "review-v7"

func newWorkflowRun(task job.ReviewJob, target pullrequest.Target, config setting.RepoConfig, modelPolicyHash string, startedAt time.Time, snapshotObservedAt time.Time) (reviewworkflow.Run, error) {
	configHash, err := hashJSON(config)
	if err != nil {
		return reviewworkflow.Run{}, fmt.Errorf("리뷰 설정 hash를 만들지 못했습니다: %w", err)
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
		ProgressMarker:     strings.TrimSpace(task.ProgressMarker),
		Trigger:            task.Trigger,
		Status:             reviewworkflow.RunStatusPlanning,
		SnapshotObservedAt: snapshotObservedAt,
		SnapshotOrderKey:   task.SnapshotOrderKey,
		StartedAt:          startedAt,
		HeartbeatAt:        startedAt,
	}, nil
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
