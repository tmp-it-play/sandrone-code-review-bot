package progresscheck

import (
	"context"
	"strings"

	"github.com/it-play/sandrone-code-review-bot/internal/core/progresscomment"
	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

type Service struct {
	deps Dependencies
}

func New(deps Dependencies) *Service {
	return &Service{deps: deps}
}

func (s *Service) Show(ctx context.Context, target pullrequest.Target, marker string, message string) error {
	marker = strings.TrimSpace(marker)
	title := progresscomment.CheckTitle(message)
	if marker == "" || title == "" {
		return nil
	}
	checkRunID, found, err := s.deps.Runs.ProgressCheckRun(ctx, marker)
	if err != nil {
		return err
	}
	if found {
		return s.deps.Checks.UpdateProgressCheck(ctx, target, checkRunID, title)
	}
	key, valid := reviewworkflow.ProgressMarkerKey(marker)
	if !valid || strings.TrimSpace(target.HeadSHA) == "" {
		return nil
	}
	createdID, err := s.deps.Checks.CreateProgressCheck(ctx, target, key, title)
	if err != nil {
		return err
	}
	return s.deps.Runs.SaveProgressCheckRun(ctx, marker, createdID)
}

func (s *Service) Complete(ctx context.Context, target pullrequest.Target, markers []string, conclusion progresscomment.CheckConclusion, message string) {
	title := progresscomment.CheckTitle(message)
	completed := make(map[int64]struct{})
	for _, marker := range markers {
		marker = strings.TrimSpace(marker)
		if marker == "" {
			continue
		}
		checkRunID, found, err := s.deps.Runs.ProgressCheckRun(ctx, marker)
		if err != nil {
			s.deps.Logger.Warn("진행 체크를 찾지 못했습니다", "target", target.Reference(), "marker", marker, "error", err)
			continue
		}
		if !found {
			continue
		}
		if _, done := completed[checkRunID]; done {
			continue
		}
		completed[checkRunID] = struct{}{}
		if err := s.deps.Checks.CompleteProgressCheck(ctx, target, checkRunID, conclusion, title); err != nil {
			s.deps.Logger.Warn("진행 체크를 완료하지 못했습니다", "target", target.Reference(), "check_run", checkRunID, "error", err)
		}
	}
}
