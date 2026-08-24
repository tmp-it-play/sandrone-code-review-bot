package handlecommand

import (
	"github.com/it-play/sandrone-code-review-bot/internal/core/job"
)

func commandMarker(request Request) string {
	return "<!-- sandrone-command:" + commandOperationKey(request) + " -->"
}

func commandOperationKey(request Request) string {
	return job.OperationKey(string(request.Command.Kind), request.Target, request.RequestIdentity, request.Command.CommentID, request.Command.InThread)
}

func commandOrderKey(request Request) string {
	return job.OrderKey(request.Command.CommentID, request.Command.InThread)
}
