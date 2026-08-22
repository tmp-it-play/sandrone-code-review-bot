package mapper

import (
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql/model"
	"github.com/it-play/sandrone-code-review-bot/internal/core/installation"
)

func ToInstallationModel(entry installation.Installation) model.Installation {
	return model.Installation{
		ID:          entry.ID,
		Account:     entry.Account,
		AccountType: entry.AccountType,
		Selection:   entry.Selection,
		InstalledAt: entry.InstalledAt,
	}
}

func ToRepositoryModel(entry installation.Repository) model.Repository {
	return model.Repository{
		InstallationID: entry.InstallationID,
		Owner:          entry.Owner,
		Name:           entry.Name,
		Private:        entry.Private,
	}
}

func ToRepository(entry model.Repository) installation.Repository {
	return installation.Repository{
		InstallationID: entry.InstallationID,
		Owner:          entry.Owner,
		Name:           entry.Name,
		Private:        entry.Private,
	}
}
