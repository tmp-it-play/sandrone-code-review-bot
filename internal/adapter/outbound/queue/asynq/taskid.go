package asynq

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"

	"github.com/it-play/sandrone-code-review-bot/internal/core/job"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
)

func deterministicTaskID(taskType string, payload any) string {
	var target pullrequest.Target
	identity := ""
	switch value := payload.(type) {
	case job.ReviewJob:
		target = value.Target
		identity = jobIdentity(value.RequestIdentity, value.CommentID, value.InThread)
	case job.SummaryJob:
		target = value.Target
		identity = jobIdentity(value.RequestIdentity, value.CommentID, value.InThread)
	case job.ReplyJob:
		target = value.Target
		identity = jobIdentity(value.RequestIdentity, value.CommentID, value.InThread)
	}
	if identity == "" {
		return ""
	}
	hash := sha256.New()
	for _, part := range []string{
		taskType,
		strconv.FormatInt(target.InstallationID, 10),
		target.Owner,
		target.Repository,
		strconv.Itoa(target.Number),
		identity,
	} {
		hash.Write([]byte(strconv.Itoa(len(part))))
		hash.Write([]byte{0})
		hash.Write([]byte(part))
	}
	return "sandrone-" + hex.EncodeToString(hash.Sum(nil))
}

func jobIdentity(requestIdentity string, commentID int64, inThread bool) string {
	if commentID > 0 {
		scope := "issue"
		if inThread {
			scope = "review"
		}
		return scope + ":comment:" + strconv.FormatInt(commentID, 10)
	}
	if requestIdentity != "" {
		return requestIdentity
	}
	return ""
}
