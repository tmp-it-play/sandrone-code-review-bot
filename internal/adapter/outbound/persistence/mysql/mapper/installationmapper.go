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
