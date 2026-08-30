package reviewpublication

import (
	"context"

	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
)

func (r *Reconciler) rebindNoopHeadChange(ctx context.Context, target pullrequest.Target, current pullrequest.PullRequest) (pullrequest.Target, bool, error) {
	if target.BaseSHA != current.BaseSHA {
		return target, false, nil
	}
	if target.HeadSHA == current.HeadSHA {
		target.BaseRef = current.BaseRef
		return target, true, nil
	}
	if target.HeadSHA == "" || current.HeadSHA == "" {
		return target, false, nil
	}
	files, err := r.deps.Source.ChangedFilesBetween(ctx, target, target.HeadSHA, current.HeadSHA)
	if err != nil {
		return target, false, err
	}
	if len(files) != 0 {
		return target, false, nil
	}
	previousHeadSHA := target.HeadSHA
	target.HeadSHA = current.HeadSHA
	target.BaseRef = current.BaseRef
	r.deps.Logger.Info("파일 변경이 없는 head 이동을 게시 조정 대상에 반영합니다", "target", target.Reference(), "previous_head", previousHeadSHA, "head", current.HeadSHA)
	return target, true, nil
}
