package mysql

import (
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/reviewworkflow"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func requireRunLease(transaction *gorm.DB, runID uint64, leaseToken string) error {
	var run model.ReviewRun
	if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, runID).Error; err != nil {
		return err
	}
	if err := validateRunLease(transaction, run, leaseToken); err != nil {
		return err
	}
	_, err := requireLatestRun(transaction, run)
	return err
}

func validateRunLease(transaction *gorm.DB, run model.ReviewRun, leaseToken string) error {
	if reviewworkflow.RunStatus(run.Status).IsTerminal() || run.LeaseToken == "" || run.LeaseToken != leaseToken || run.LeaseExpiresAt == nil {
		return reviewworkflow.ErrRunLeased
	}
	now, err := databaseTime(transaction)
	if err != nil {
		return err
	}
	if !run.LeaseExpiresAt.After(now) {
		return reviewworkflow.ErrRunLeased
	}
	return nil
}

func requireLatestRun(transaction *gorm.DB, run model.ReviewRun) (model.PullRequestState, error) {
	var state model.PullRequestState
	if err := transaction.Clauses(clause.Locking{Strength: "UPDATE"}).Where("owner = ? AND repository = ? AND number = ?", run.Owner, run.Repository, run.Number).First(&state).Error; err != nil {
		return model.PullRequestState{}, err
	}
	if state.LatestRunID != run.ID || state.LatestHeadSHA != run.HeadSHA {
		return model.PullRequestState{}, reviewworkflow.ErrRunSuperseded
	}
	return state, nil
}

func terminalRunStatuses() []string {
	return []string{
		string(reviewworkflow.RunStatusComplete),
		string(reviewworkflow.RunStatusPartial),
		string(reviewworkflow.RunStatusFailed),
		string(reviewworkflow.RunStatusSuperseded),
		string(reviewworkflow.RunStatusCancelled),
		string(reviewworkflow.RunStatusSkipped),
	}
}
