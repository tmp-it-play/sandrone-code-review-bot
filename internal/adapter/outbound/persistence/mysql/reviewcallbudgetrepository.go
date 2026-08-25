package mysql

import (
	"context"
	"errors"
	"fmt"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *ReviewExecutionStore) ReserveExternalCall(ctx context.Context, reservation reviewworkflow.ExternalCallReservation) (bool, error) {
	if reservation.Limit < 1 {
		return false, nil
	}
	if err := validateExternalCallReservation(reservation); err != nil {
		return false, fmt.Errorf("외부 호출 예약이 유효하지 않습니다: %w", err)
	}
	reserved := false
	err := r.database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		if err := requireRunLease(transaction, reservation.RunID, reservation.RunLeaseToken); err != nil {
			return err
		}
		var run model.ReviewRun
		if err := transaction.First(&run, reservation.RunID).Error; err != nil {
			return err
		}
		if run.ExternalCalls >= reservation.Limit {
			return nil
		}
		var unit model.ReviewUnit
		if reservation.HasUnit() {
			if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("review_run_id = ? AND unit_hash = ? AND status = ? AND lease_token = ? AND lease_expires_at > CURRENT_TIMESTAMP(6)", reservation.RunID, reservation.UnitHash, string(reviewworkflow.UnitStatusRunning), reservation.UnitLeaseToken).
				First(&unit).Error; err != nil {
				return err
			}
		}
		updatedRun := transaction.Model(&model.ReviewRun{}).
			Where("id = ? AND lease_token = ? AND COALESCE(external_calls, 0) < ?", reservation.RunID, reservation.RunLeaseToken, reservation.Limit).
			Updates(map[string]any{
				"external_calls": gorm.Expr("GREATEST(COALESCE(external_calls, 0), 0) + 1"),
				"heartbeat_at":   reservation.HeartbeatAt,
				"lease_expires_at": gorm.Expr(
					"CASE WHEN lease_expires_at > ? THEN lease_expires_at ELSE ? END",
					reservation.RunLeaseExpiresAt,
					reservation.RunLeaseExpiresAt,
				),
			})
		if updatedRun.Error != nil {
			return updatedRun.Error
		}
		if updatedRun.RowsAffected != 1 {
			return nil
		}
		if reservation.HasUnit() {
			updatedUnit := transaction.Model(&model.ReviewUnit{}).
				Where("id = ? AND status = ? AND lease_token = ? AND lease_expires_at > CURRENT_TIMESTAMP(6)", unit.ID, string(reviewworkflow.UnitStatusRunning), reservation.UnitLeaseToken).
				Updates(map[string]any{
					"heartbeat_at": reservation.HeartbeatAt,
					"lease_expires_at": gorm.Expr(
						"CASE WHEN lease_expires_at > ? THEN lease_expires_at ELSE ? END",
						reservation.UnitLeaseExpiresAt,
						reservation.UnitLeaseExpiresAt,
					),
				})
			if updatedUnit.Error != nil {
				return updatedUnit.Error
			}
			if updatedUnit.RowsAffected != 1 {
				return errors.New("리뷰 unit 외부 호출 lease 갱신이 충돌했습니다")
			}
		}
		reserved = true
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("리뷰 실행 외부 호출 예산을 예약하지 못했습니다: %w", err)
	}
	return reserved, nil
}

func validateExternalCallReservation(reservation reviewworkflow.ExternalCallReservation) error {
	if reservation.RunID == 0 || reservation.RunLeaseToken == "" {
		return errors.New("리뷰 실행과 lease가 필요합니다")
	}
	if reservation.HeartbeatAt.IsZero() || !reservation.RunLeaseExpiresAt.After(reservation.HeartbeatAt) {
		return errors.New("리뷰 실행 heartbeat와 lease 만료 시각이 유효하지 않습니다")
	}
	if !reservation.HasUnit() {
		return nil
	}
	if reservation.UnitHash == "" || reservation.UnitLeaseToken == "" || !reservation.UnitLeaseExpiresAt.After(reservation.HeartbeatAt) {
		return errors.New("리뷰 unit 식별자와 lease 만료 시각이 유효하지 않습니다")
	}
	return nil
}
