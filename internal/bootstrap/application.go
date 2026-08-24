package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/hibiken/asynq"
	inboundcommand "github.com/it-play/sandrone-code-review-bot/internal/adapter/inbound/command"
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/inbound/dashboard"
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/inbound/httpapi"
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/inbound/maintenance"
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/inbound/webhook"
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/inbound/worker"
	redisadapter "github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/cache/redis"
	githubadapter "github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/forge/github"
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/forge/github/markdown"
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/llm/chain"
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/masking"
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/observability"
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql"
	queueadapter "github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/queue/asynq"
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/settings"
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/system"
	"github.com/it-play/sandrone-code-review-bot/internal/core/parsing"
	"github.com/it-play/sandrone-code-review-bot/internal/core/usecase/handlecommand"
	"github.com/it-play/sandrone-code-review-bot/internal/core/usecase/replythread"
	"github.com/it-play/sandrone-code-review-bot/internal/core/usecase/reviewpullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/usecase/summarizepullrequest"
	"github.com/redis/go-redis/v9"
)

const botName = "sandrone-code-review-bot"
const asynqShutdownTimeout = 2 * time.Minute

type Application struct {
	logger              *slog.Logger
	httpServer          *http.Server
	asynqServer         *asynq.Server
	asynqMux            *asynq.ServeMux
	queueClient         *queueadapter.Client
	inspector           *asynq.Inspector
	cache               *redis.Client
	reviewRetention     *maintenance.ReviewRetentionWorker
	reviewPublication   *maintenance.ReviewPublicationWorker
	webhookInbox        *maintenance.WebhookInboxWorker
	webhookRecovery     *maintenance.WebhookDeliveryRecoveryWorker
	occurrenceBootstrap *maintenance.FindingOccurrenceBootstrapWorker
}

func NewApplication(config Config) (*Application, error) {
	logger := observability.NewLogger()
	metrics := observability.NewMetrics()
	clock := system.NewClock()
	masker := masking.NewSecretMasker()

	database, err := mysql.NewConnection(config.MySQLDSN)
	if err != nil {
		return nil, err
	}
	if err := mysql.Migrate(database); err != nil {
		return nil, err
	}

	cache := redisadapter.NewClient(config.RedisAddress, config.RedisPassword, config.RedisDatabase)
	redisOptions := asynq.RedisClientOpt{
		Addr:     config.RedisAddress,
		Password: config.RedisPassword,
		DB:       config.RedisDatabase,
	}
	queueClient := queueadapter.NewClient(asynq.NewClient(redisOptions))
	inspector := asynq.NewInspector(redisOptions)

	tokens, err := githubadapter.NewTokenSource(config.AppID, config.PrivateKey)
	if err != nil {
		return nil, err
	}
	clients := githubadapter.NewClientFactory(tokens)
	webhookDeliveryRecovery := githubadapter.NewAppWebhookDeliveryRecovery(tokens)
	pullRequests := githubadapter.NewPullRequestSource(clients)
	contents := githubadapter.NewRepositoryContent(clients)
	publisher := githubadapter.NewReviewPublisher(clients)
	threads := githubadapter.NewThreadPublisher(clients)
	reactions := githubadapter.NewReactionPublisher(clients)
	permissions := githubadapter.NewPermissionChecker(clients)
	tools := githubadapter.NewToolExecutorFactory(contents, masker, 16000)
	renderer := markdown.NewRenderer(botName)

	settingSource := settings.NewSettingSource(
		settings.NewConfigLoader(contents, logger),
		settings.NewInstructionCollector(contents, masker, logger),
	)

	reviews := mysql.NewReviewRepository(database)
	reviewWorkflows := mysql.NewReviewWorkflowRepository(database)
	pullRequestState := mysql.NewPullRequestStateRepository(database)
	findings := mysql.NewFindingRepository(database)
	occurrenceBootstrapRepository := mysql.NewFindingOccurrenceBootstrapRepository(database)
	installations := mysql.NewInstallationRepository(database)
	usageRepository := mysql.NewUsageRepository(database)
	commands := mysql.NewCommandRepository(database)
	webhookInboxRepository := mysql.NewWebhookInboxRepository(database)
	cooldown := redisadapter.NewCooldown(cache)

	completer := chain.New(buildProviders(config), cooldown, usageRepository, metrics, clock, logger, config.ProviderCooldown)
	parser := parsing.ResultParser{}

	reviewUseCase := reviewpullrequest.New(reviewpullrequest.Dependencies{
		Source:      pullRequests,
		Settings:    settingSource,
		Masker:      masker,
		Completer:   completer,
		Tools:       tools,
		Publisher:   publisher,
		Reactions:   reactions,
		Renderer:    renderer,
		Reviews:     reviews,
		Workflows:   reviewWorkflows,
		Findings:    findings,
		State:       pullRequestState,
		Clock:       clock,
		Parser:      parser,
		Logger:      logger,
		Retention:   config.ReviewRetention,
		LLMMaxCalls: config.LLMMaxCalls,
	})
	reviewRetention := maintenance.NewReviewRetentionWorker(reviewWorkflows, clock, logger, config.ReviewRetention)
	reviewPublication := maintenance.NewReviewPublicationWorker(reviewWorkflows, pullRequests, publisher, reviews, clock, logger, config.ReviewRetention)
	occurrenceBootstrap := maintenance.NewFindingOccurrenceBootstrapWorker(occurrenceBootstrapRepository, logger)
	summaryUseCase := summarizepullrequest.New(summarizepullrequest.Dependencies{
		Source:       pullRequests,
		Settings:     settingSource,
		Masker:       masker,
		Completer:    completer,
		Publisher:    publisher,
		Renderer:     renderer,
		Reviews:      reviews,
		Publications: reviewWorkflows,
		Clock:        clock,
		Parser:       parser,
		Logger:       logger,
		Retention:    config.ReviewRetention,
		LLMMaxCalls:  config.LLMMaxCalls,
	})
	replyUseCase := replythread.New(replythread.Dependencies{
		Source:       pullRequests,
		Settings:     settingSource,
		Masker:       masker,
		Completer:    completer,
		Threads:      threads,
		Renderer:     renderer,
		Publications: reviewWorkflows,
		Clock:        clock,
		Logger:       logger,
		Retention:    config.ReviewRetention,
		LLMMaxCalls:  config.LLMMaxCalls,
	})
	commandUseCase := handlecommand.New(handlecommand.Dependencies{
		Permissions: permissions,
		Reactions:   reactions,
		Threads:     threads,
		Publisher:   publisher,
		Renderer:    renderer,
		Queue:       queueClient,
		Commands:    commands,
		Clock:       clock,
		Logger:      logger,
	})

	router := webhook.NewEventRouter(queueClient, commandUseCase, inboundcommand.NewParser(botName), installations)
	webhookInbox := maintenance.NewWebhookInboxWorker(webhookInboxRepository, router, clock, masker, metrics, logger, config.DeliveryRetention)
	webhookRecovery := maintenance.NewWebhookDeliveryRecoveryWorker(webhookDeliveryRecovery, clock, logger)
	webhookHandler := webhook.NewHandler(config.WebhookSecret, webhookInboxRepository, clock, logger)

	dashboardServer, err := dashboard.NewServer(dashboard.Dependencies{
		Reviews:       reviews,
		Findings:      findings,
		Installations: installations,
		Usage:         usageRepository,
		Commands:      commands,
		Cooldown:      cooldown,
		Queue:         queueClient,
		Inspector:     inspector,
		BasePath:      config.BasePath,
		Credentials:   dashboard.Credentials{Username: config.DashboardUsername, Password: config.DashboardPassword},
		Sessions:      dashboard.NewSessionStore(config.DashboardSecret, 12*time.Hour),
		ProviderOrder: config.ProviderOrder,
		TemplateDir:   config.TemplateDirectory,
		StaticDir:     config.StaticDirectory,
		Logger:        logger,
	})
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	mux.Handle("/webhook", webhookHandler)
	mux.Handle("/healthz", httpapi.NewHealthHandler(database, cache))
	mux.Handle("/metrics", httpapi.NewMetricsHandler(metrics, config.MetricsToken))
	mux.HandleFunc("GET /{$}", func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, config.BasePath+"/dashboard", http.StatusSeeOther)
	})
	dashboardServer.Register(mux)

	handler := mount(config.BasePath, mux)

	asynqMux := asynq.NewServeMux()
	asynqMux.Handle(queueadapter.TaskReview, worker.NewReviewHandler(reviewUseCase, metrics))
	asynqMux.Handle(queueadapter.TaskSummary, worker.NewSummaryHandler(summaryUseCase, metrics))
	asynqMux.Handle(queueadapter.TaskReply, worker.NewReplyHandler(replyUseCase, metrics))

	asynqServer := asynq.NewServer(redisOptions, asynq.Config{
		Concurrency:     config.WorkerConcurrency,
		ShutdownTimeout: asynqShutdownTimeout,
		RetryDelayFunc: asynq.RetryDelayFunc(func(attempt int, _ error, _ *asynq.Task) time.Duration {
			return retryDelay(attempt)
		}),
		Logger: newQueueLogger(logger),
	})

	return &Application{
		logger:              logger,
		httpServer:          &http.Server{Addr: config.HTTPAddress, Handler: handler, ReadHeaderTimeout: 10 * time.Second},
		asynqServer:         asynqServer,
		asynqMux:            asynqMux,
		queueClient:         queueClient,
		inspector:           inspector,
		cache:               cache,
		reviewRetention:     reviewRetention,
		reviewPublication:   reviewPublication,
		webhookInbox:        webhookInbox,
		webhookRecovery:     webhookRecovery,
		occurrenceBootstrap: occurrenceBootstrap,
	}, nil
}

func (a *Application) Run(ctx context.Context) error {
	if err := a.asynqServer.Start(a.asynqMux); err != nil {
		return fmt.Errorf("워커를 시작하지 못했습니다: %w", err)
	}
	errs := make(chan error, 1)
	go func() {
		a.logger.Info("HTTP 서버를 시작합니다", "address", a.httpServer.Addr)
		if err := a.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs <- err
			return
		}
		errs <- nil
	}()
	maintenanceContext, stopMaintenance := context.WithCancel(ctx)
	var maintenanceWorkers sync.WaitGroup
	for _, run := range []func(context.Context){
		a.reviewRetention.Run,
		a.reviewPublication.Run,
		a.webhookInbox.Run,
		a.webhookRecovery.Run,
		a.occurrenceBootstrap.Run,
	} {
		maintenanceWorkers.Add(1)
		go func(run func(context.Context)) {
			defer maintenanceWorkers.Done()
			run(maintenanceContext)
		}(run)
	}

	select {
	case <-ctx.Done():
	case err := <-errs:
		stopMaintenance()
		maintenanceWorkers.Wait()
		a.shutdown()
		return err
	}
	stopMaintenance()
	maintenanceWorkers.Wait()
	a.shutdown()
	return nil
}

func (a *Application) shutdown() {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := a.httpServer.Shutdown(shutdownCtx); err != nil {
		a.logger.Warn("HTTP 서버를 정상 종료하지 못했습니다", "error", err)
	}
	a.asynqServer.Shutdown()
	if err := a.queueClient.Close(); err != nil {
		a.logger.Warn("큐 클라이언트를 닫지 못했습니다", "error", err)
	}
	if err := a.inspector.Close(); err != nil {
		a.logger.Warn("큐 인스펙터를 닫지 못했습니다", "error", err)
	}
	if err := a.cache.Close(); err != nil {
		a.logger.Warn("Redis 연결을 닫지 못했습니다", "error", err)
	}
	a.logger.Info("종료했습니다")
}

func retryDelay(attempt int) time.Duration {
	delays := []time.Duration{
		1 * time.Minute,
		5 * time.Minute,
		15 * time.Minute,
		30 * time.Minute,
		1 * time.Hour,
		2 * time.Hour,
	}
	if attempt < 1 {
		attempt = 1
	}
	if attempt > len(delays) {
		attempt = len(delays)
	}
	return delays[attempt-1]
}
