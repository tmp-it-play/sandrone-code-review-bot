package dashboard

import (
	"log/slog"

	"github.com/hibiken/asynq"
	"github.com/it-play/sandrone-code-review-bot/internal/core/port/outbound"
)

type Dependencies struct {
	Reviews       outbound.ReviewRepository
	Findings      outbound.FindingRepository
	Installations outbound.InstallationRepository
	Usage         outbound.UsageRepository
	Commands      outbound.CommandRepository
	Cooldown      outbound.Cooldown
	Queue         outbound.Queue
	Inspector     *asynq.Inspector
	Credentials   Credentials
	Sessions      *SessionStore
	ProviderOrder []string
	BasePath      string
	TemplateDir   string
	StaticDir     string
	Logger        *slog.Logger
}
