package mysql

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/progresscomment"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var errProgressCommentOwnershipRace = errors.New("진행 코멘트 소유권 생성 경합")

func ensureProgressCommentOwnership(transaction *gorm.DB, run model.ReviewRun, currentAt time.Time) error {
	if run.ProgressMarker == "" {
		return nil
	}
	if _, valid := reviewworkflow.ProgressMarkerKey(run.ProgressMarker); !valid {
		return fmt.Errorf("리뷰 실행의 진행 marker가 올바르지 않습니다")
	}
	var ownership model.ProgressCommentOwnership
	ownershipErr := transaction.Where("marker = ?", run.ProgressMarker).First(&ownership).Error
	if errors.Is(ownershipErr, gorm.ErrRecordNotFound) {
		ownership = model.ProgressCommentOwnership{
			Marker:               run.ProgressMarker,
			ReviewRunID:          run.ID,
			InstallationID:       run.InstallationID,
			Owner:                run.Owner,
			Repository:           run.Repository,
			Number:               run.Number,
			ProgressMessageTheme: string(progresscomment.ThemeProgramming),
			CreateNotBefore:      timePointer(currentAt),
			NextRefreshAt:        timePointer(currentAt.Add(progresscomment.RefreshInterval)),
			RefreshExpiresAt:     timePointer(currentAt.Add(progresscomment.RefreshLifetime)),
			CreatedAt:            currentAt,
			UpdatedAt:            currentAt,
		}
		created := transaction.Clauses(clause.OnConflict{DoNothing: true}).Create(&ownership)
		if created.Error != nil {
			return created.Error
		}
		if created.RowsAffected != 1 {
			return errProgressCommentOwnershipRace
		}
		return nil
	}
	if ownershipErr != nil {
		return ownershipErr
	}
	if ownership.ReviewRunID > run.ID {
		return nil
	}
	if ownership.ReviewRunID == run.ID {
		if err := requireProgressCommentOwnership(transaction, run, currentAt); err != nil {
			return err
		}
		if progressCommentRefreshActive(run) {
			return activateProgressCommentRefreshForUpdate(transaction, run, currentAt)
		}
		return nil
	}
	blockedUntil, err := progressCommentOwnerBlockedUntil(transaction, ownership.ReviewRunID, currentAt)
	if err != nil {
		return err
	}
	if !blockedUntil.IsZero() {
		return &reviewworkflow.LeaseConflict{Cause: reviewworkflow.ErrProgressCommentLeased, Until: blockedUntil}
	}
	var locked model.ProgressCommentOwnership
	if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).Where("marker = ?", run.ProgressMarker).First(&locked).Error; err != nil {
		return err
	}
	if locked.ReviewRunID != ownership.ReviewRunID {
		return reviewworkflow.ErrProgressCommentLeased
	}
	if locked.ReviewRunID != 0 && locked.CleanupLeaseToken != "" && locked.CleanupLeaseExpiresAt != nil && locked.CleanupLeaseExpiresAt.After(currentAt) {
		return &reviewworkflow.LeaseConflict{Cause: reviewworkflow.ErrProgressCommentLeased, Until: *locked.CleanupLeaseExpiresAt}
	}
	updates := map[string]any{
		"review_run_id":      run.ID,
		"installation_id":    run.InstallationID,
		"owner":              run.Owner,
		"repository":         run.Repository,
		"number":             run.Number,
		"refresh_expires_at": currentAt.Add(progresscomment.RefreshLifetime),
		"updated_at":         currentAt,
	}
	if progressCommentRefreshActive(run) {
		updates["next_refresh_at"] = currentAt
		updates["refresh_stop_requested_at"] = nil
	} else {
		updates["next_refresh_at"] = nil
		updates["refresh_stop_requested_at"] = currentAt
	}
	if locked.ReviewRunID != 0 {
		updates["refresh_sequence"] = 0
		updates["cleanup_lease_token"] = ""
		updates["cleanup_lease_expires_at"] = nil
	}
	updated := transaction.Model(&model.ProgressCommentOwnership{}).
		Where("marker = ? AND review_run_id = ?", locked.Marker, locked.ReviewRunID).
		Updates(updates)
	if updated.Error != nil {
		return updated.Error
	}
	if updated.RowsAffected != 1 {
		return reviewworkflow.ErrProgressCommentLeased
	}
	return nil
}

func requireProgressCommentOwnership(transaction *gorm.DB, run model.ReviewRun, currentAt time.Time) error {
	if run.ProgressMarker == "" {
		return nil
	}
	var ownership model.ProgressCommentOwnership
	if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).Where("marker = ?", run.ProgressMarker).First(&ownership).Error; err != nil {
		return err
	}
	if ownership.ReviewRunID > run.ID {
		return reviewworkflow.ErrRunSuperseded
	}
	if ownership.ReviewRunID < run.ID {
		return reviewworkflow.ErrProgressCommentLeased
	}
	if ownership.CleanupLeaseToken != "" && ownership.CleanupLeaseExpiresAt != nil && ownership.CleanupLeaseExpiresAt.After(currentAt) {
		return &reviewworkflow.LeaseConflict{Cause: reviewworkflow.ErrProgressCommentLeased, Until: *ownership.CleanupLeaseExpiresAt}
	}
	return nil
}

func progressCommentOwnerBlockedUntil(transaction *gorm.DB, runID uint64, currentAt time.Time) (time.Time, error) {
	var run model.ReviewRun
	err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id", "owner", "repository", "number", "status", "terminal_at", "lease_expires_at").First(&run, runID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	blockedUntil := time.Time{}
	if run.TerminalAt != nil {
		blockedUntil = run.TerminalAt.Add(reviewworkflow.PublicationInvalidationFenceDelay)
	} else if run.LeaseExpiresAt != nil {
		blockedUntil = *run.LeaseExpiresAt
	}
	if reviewworkflow.RunStatus(run.Status) == reviewworkflow.RunStatusPublishing {
		var state model.PullRequestState
		stateErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).Select("publishing_run_id", "publishing_lease_expires_at").Where("owner = ? AND repository = ? AND number = ?", run.Owner, run.Repository, run.Number).First(&state).Error
		if stateErr != nil && !errors.Is(stateErr, gorm.ErrRecordNotFound) {
			return time.Time{}, stateErr
		}
		if stateErr == nil && state.PublishingRunID == run.ID && state.PublishingLeaseExpiresAt != nil && state.PublishingLeaseExpiresAt.After(blockedUntil) {
			blockedUntil = *state.PublishingLeaseExpiresAt
		}
	}
	if blockedUntil.After(currentAt) {
		return blockedUntil, nil
	}
	return time.Time{}, nil
}

func claimProgressCommentCleanup(transaction *gorm.DB, marker string, runID uint64, leaseToken string, claimedAt time.Time, leaseExpiresAt time.Time) (bool, error) {
	if marker == "" {
		return false, nil
	}
	var ownership model.ProgressCommentOwnership
	err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).Where("marker = ?", marker).First(&ownership).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	currentAt, err := databaseTime(transaction)
	if err != nil {
		return false, err
	}
	if !leaseExpiresAt.After(currentAt) {
		return false, reviewworkflow.ErrProgressCommentLeased
	}
	owned := ownership.ReviewRunID == runID
	if ownership.CleanupLeaseToken != "" && ownership.CleanupLeaseToken != leaseToken && ownership.CleanupLeaseExpiresAt != nil && ownership.CleanupLeaseExpiresAt.After(currentAt) {
		return false, &reviewworkflow.LeaseConflict{Cause: reviewworkflow.ErrProgressCommentLeased, Until: *ownership.CleanupLeaseExpiresAt}
	}
	updated := transaction.Model(&model.ProgressCommentOwnership{}).
		Where("marker = ? AND review_run_id = ?", marker, ownership.ReviewRunID).
		Updates(map[string]any{
			"cleanup_lease_token":      leaseToken,
			"cleanup_lease_expires_at": leaseExpiresAt,
			"updated_at":               claimedAt,
		})
	if updated.Error != nil {
		return false, updated.Error
	}
	if updated.RowsAffected != 1 {
		return false, reviewworkflow.ErrProgressCommentLeased
	}
	return owned, nil
}

func ownsProgressCommentForUpdate(transaction *gorm.DB, runID uint64, marker string) (bool, error) {
	if runID == 0 || marker == "" {
		return false, nil
	}
	var ownership model.ProgressCommentOwnership
	err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).Where("marker = ?", marker).First(&ownership).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return ownership.ReviewRunID == runID, nil
}

func releaseProgressCommentCleanup(transaction *gorm.DB, leaseToken string, releasedAt time.Time) error {
	if leaseToken == "" {
		return nil
	}
	return transaction.Model(&model.ProgressCommentOwnership{}).
		Where("cleanup_lease_token = ?", leaseToken).
		Updates(map[string]any{
			"cleanup_lease_token":      "",
			"cleanup_lease_expires_at": nil,
			"updated_at":               releasedAt,
		}).Error
}

func stopProgressCommentRefreshForUpdate(transaction *gorm.DB, run model.ReviewRun, currentAt time.Time) error {
	if run.ProgressMarker == "" {
		return nil
	}
	var ownership model.ProgressCommentOwnership
	err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).Where("marker = ?", run.ProgressMarker).First(&ownership).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if ownership.ReviewRunID != run.ID {
		return nil
	}
	return transaction.Model(&model.ProgressCommentOwnership{}).
		Where("marker = ? AND review_run_id = ?", run.ProgressMarker, run.ID).
		Updates(map[string]any{
			"next_refresh_at":           nil,
			"refresh_stop_requested_at": currentAt,
			"updated_at":                currentAt,
		}).Error
}

func activateProgressCommentRefreshForUpdate(transaction *gorm.DB, run model.ReviewRun, currentAt time.Time) error {
	if run.ProgressMarker == "" {
		return nil
	}
	var ownership model.ProgressCommentOwnership
	err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).Where("marker = ?", run.ProgressMarker).First(&ownership).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if ownership.ReviewRunID != run.ID {
		return nil
	}
	activeUntil := currentAt.Add(progresscomment.RefreshLifetime)
	expiresAt := ownership.RefreshExpiresAt
	if expiresAt == nil || expiresAt.Before(activeUntil) {
		expiresAt = timePointer(activeUntil)
	}
	return transaction.Model(&model.ProgressCommentOwnership{}).
		Where("marker = ? AND review_run_id = ?", run.ProgressMarker, run.ID).
		Updates(map[string]any{
			"next_refresh_at":           currentAt,
			"refresh_expires_at":        expiresAt,
			"refresh_stop_requested_at": nil,
			"updated_at":                currentAt,
		}).Error
}

func progressCommentRefreshActive(run model.ReviewRun) bool {
	status := reviewworkflow.RunStatus(run.Status)
	return !status.IsTerminal() && status != reviewworkflow.RunStatusPublishing
}

func (r *ReviewRunStore) OwnsProgressMarker(ctx context.Context, runID uint64, marker string) (bool, error) {
	if runID == 0 || marker == "" {
		return false, nil
	}
	var run model.ReviewRun
	if err := r.database.WithContext(ctx).Select("id", "run_key").First(&run, runID).Error; err != nil {
		return false, fmt.Errorf("진행 코멘트 실행을 읽지 못했습니다: %w", err)
	}
	var ownership model.ProgressCommentOwnership
	err := r.database.WithContext(ctx).Where("marker = ?", marker).First(&ownership).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return marker == reviewworkflow.ProgressMarker(run.RunKey), nil
	}
	if err != nil {
		return false, fmt.Errorf("진행 코멘트 소유권을 읽지 못했습니다: %w", err)
	}
	currentAt, err := databaseTime(r.database.WithContext(ctx))
	if err != nil {
		return false, fmt.Errorf("진행 코멘트 소유권 시간을 읽지 못했습니다: %w", err)
	}
	if ownership.CleanupLeaseToken != "" && ownership.CleanupLeaseExpiresAt != nil && ownership.CleanupLeaseExpiresAt.After(currentAt) {
		return false, &reviewworkflow.LeaseConflict{Cause: reviewworkflow.ErrProgressCommentLeased, Until: *ownership.CleanupLeaseExpiresAt}
	}
	return ownership.ReviewRunID == runID, nil
}

func (r *ReviewRunStore) ClaimProgressCommentMutation(ctx context.Context, runID uint64, marker string, claimedAt time.Time, leaseExpiresAt time.Time) (string, bool, error) {
	if _, valid := reviewworkflow.ProgressMarkerKey(marker); !valid {
		return "", false, fmt.Errorf("진행 코멘트 변경 marker가 올바르지 않습니다")
	}
	if claimedAt.IsZero() || !leaseExpiresAt.After(claimedAt) {
		return "", false, fmt.Errorf("진행 코멘트 변경 lease 시간이 올바르지 않습니다")
	}
	leaseDuration := leaseExpiresAt.Sub(claimedAt)
	leaseToken, err := newLeaseToken()
	if err != nil {
		return "", false, fmt.Errorf("진행 코멘트 변경 lease를 만들지 못했습니다: %w", err)
	}
	owned := false
	err = r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		var ownership model.ProgressCommentOwnership
		ownershipErr := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).Where("marker = ?", marker).First(&ownership).Error
		if errors.Is(ownershipErr, gorm.ErrRecordNotFound) {
			return nil
		}
		if ownershipErr != nil {
			return ownershipErr
		}
		currentAt, err := databaseTime(transaction)
		if err != nil {
			return err
		}
		effectiveLeaseExpiresAt := currentAt.Add(leaseDuration)
		owned = ownership.ReviewRunID == runID
		if runID == 0 {
			owned = ownership.RefreshStopRequestedAt == nil && ownership.NextRefreshAt != nil && ownership.RefreshExpiresAt != nil && ownership.RefreshExpiresAt.After(currentAt)
		}
		if !owned {
			return nil
		}
		if ownership.CleanupLeaseToken != "" && ownership.CleanupLeaseExpiresAt != nil && ownership.CleanupLeaseExpiresAt.After(currentAt) {
			return &reviewworkflow.LeaseConflict{Cause: reviewworkflow.ErrProgressCommentLeased, Until: *ownership.CleanupLeaseExpiresAt}
		}
		updated := transaction.Model(&model.ProgressCommentOwnership{}).
			Where("marker = ? AND review_run_id = ?", marker, ownership.ReviewRunID).
			Updates(map[string]any{
				"cleanup_lease_token":      leaseToken,
				"cleanup_lease_expires_at": effectiveLeaseExpiresAt,
				"updated_at":               currentAt,
			})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return reviewworkflow.ErrProgressCommentLeased
		}
		return nil
	})
	if err != nil {
		return "", false, fmt.Errorf("진행 코멘트 변경 lease를 얻지 못했습니다: %w", err)
	}
	if !owned {
		return "", false, nil
	}
	return leaseToken, true, nil
}

func (r *ReviewRunStore) CompleteProgressCommentMutation(ctx context.Context, marker string, leaseToken string, completedAt time.Time) error {
	updated := r.database.WithContext(ctx).Model(&model.ProgressCommentOwnership{}).
		Where("marker = ? AND cleanup_lease_token = ?", marker, leaseToken).
		Updates(map[string]any{
			"next_refresh_at":          gorm.Expr("CASE WHEN refresh_stop_requested_at IS NULL THEN ? ELSE NULL END", completedAt.Add(progresscomment.RefreshInterval)),
			"cleanup_lease_token":      "",
			"cleanup_lease_expires_at": nil,
			"updated_at":               completedAt,
		})
	if updated.Error != nil {
		return fmt.Errorf("진행 코멘트 변경 완료를 저장하지 못했습니다: %w", updated.Error)
	}
	if updated.RowsAffected != 1 {
		return reviewworkflow.ErrProgressCommentLeased
	}
	return nil
}

func (r *ReviewRunStore) FenceProgressCommentMutation(ctx context.Context, marker string, leaseToken string, failedAt time.Time, uncertainUntil time.Time) error {
	if !uncertainUntil.After(failedAt) {
		return r.CompleteProgressCommentMutation(ctx, marker, leaseToken, failedAt)
	}
	updated := r.database.WithContext(ctx).Model(&model.ProgressCommentOwnership{}).
		Where("marker = ? AND cleanup_lease_token = ?", marker, leaseToken).
		Updates(map[string]any{
			"next_refresh_at":          gorm.Expr("CASE WHEN refresh_stop_requested_at IS NULL THEN ? ELSE NULL END", uncertainUntil),
			"cleanup_lease_expires_at": uncertainUntil,
			"updated_at":               failedAt,
		})
	if updated.Error != nil {
		return fmt.Errorf("불확실한 진행 코멘트 변경을 저장하지 못했습니다: %w", updated.Error)
	}
	if updated.RowsAffected != 1 {
		return reviewworkflow.ErrProgressCommentLeased
	}
	return nil
}
