package reviewpullrequest

import (
	"context"
	"sync"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
)

type progressSession struct {
	commentID       int64
	marker          string
	handoffMarker   string
	runID           uint64
	createMissing   bool
	commentMu       sync.RWMutex
	stateMu         sync.Mutex
	createNotBefore time.Time
	uncertainUntil  time.Time
}

func (s *progressSession) TerminalFinalization() *reviewworkflow.TerminalProgressFinalization {
	if s == nil {
		return nil
	}
	marker := s.marker
	if s.handoffMarker != "" {
		marker = s.handoffMarker
	}
	return reviewworkflow.NewTerminalProgressFinalization(marker)
}

func (s *progressSession) CommentID() int64 {
	if s == nil {
		return 0
	}
	s.commentMu.RLock()
	defer s.commentMu.RUnlock()
	return s.commentID
}

func (s *progressSession) SetCommentID(commentID int64) {
	if s == nil {
		return
	}
	s.commentMu.Lock()
	s.commentID = commentID
	s.commentMu.Unlock()
}

func (s *progressSession) RunID() uint64 {
	if s == nil {
		return 0
	}
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return s.runID
}

func (s *progressSession) SetRunID(runID uint64) {
	if s == nil {
		return
	}
	s.stateMu.Lock()
	s.runID = runID
	s.stateMu.Unlock()
}

func (s *progressSession) Marker() string {
	if s == nil {
		return ""
	}
	return s.marker
}

func (s *progressSession) Markers() []string {
	if s == nil {
		return nil
	}
	markers := make([]string, 0, 2)
	if s.marker != "" {
		markers = append(markers, s.marker)
	}
	if s.handoffMarker != "" && s.handoffMarker != s.marker {
		markers = append(markers, s.handoffMarker)
	}
	return markers
}

func (s *progressSession) CanCreate() bool {
	return s != nil && s.createMissing
}

func (s *progressSession) ResolveDiscoveredComment() {
	if s == nil {
		return
	}
	s.stateMu.Lock()
	s.createNotBefore = time.Time{}
	s.stateMu.Unlock()
}

func (s *progressSession) MarkMutationUncertain(until time.Time) {
	if s == nil {
		return
	}
	s.stateMu.Lock()
	if until.After(s.uncertainUntil) {
		s.uncertainUntil = until
	}
	s.stateMu.Unlock()
}

func (s *progressSession) WaitForMutationFence(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.stateMu.Lock()
	until := s.uncertainUntil
	if s.createNotBefore.After(until) {
		until = s.createNotBefore
	}
	s.stateMu.Unlock()
	delay := time.Until(until)
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-timer.C:
		return nil
	}
}

func (s *progressSession) Stop() {
}
