package bootstrap

import (
	"database/sql"
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
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/health"
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/llm/chain"
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/masking"
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/observability"
	prometheusmetrics "github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/observability/prometheus"
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/persistence/mysql"
	queueadapter "github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/queue/asynq"
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/settings"
	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/system"
	"github.com/it-play/sandrone-code-review-bot/internal/core/parsing"
	"github.com/it-play/sandrone-code-review-bot/internal/core/usecase/handlecommand"
	"github.com/it-play/sandrone-code-review-bot/internal/core/usecase/progresscheck"
	"github.com/it-play/sandrone-code-review-bot/internal/core/usecase/replythread"
	"github.com/it-play/sandrone-code-review-bot/internal/core/usecase/reviewenqueue"
	"github.com/it-play/sandrone-code-review-bot/internal/core/usecase/reviewpullrequest"
	"github.com/it-play/sandrone-code-review-bot/internal/core/usecase/summarizepullrequest"
	"github.com/redis/go-redis/v9"
)

const botName = "sandrone-code-review-bot"
const asynqShutdownTimeout = 2 * time.Minute

type Application struct {
	logger              *slog.Logger
	database            *sql.DB
	httpServer          *http.Server
	asynqServer         *asynq.Server
	asynqMux            *asynq.ServeMux
	queueClient         *queueadapter.Client
	inspector           *asynq.Inspector
	cache               *redis.Client
	reviewRetention     *maintenance.ReviewRetentionWorker
	reviewPublication   *maintenance.ReviewPublicationWorker
	progressComment     *maintenance.ProgressCommentWorker
	webhookInbox        *maintenance.WebhookInboxWorker
	webhookRecovery     *maintenance.WebhookDeliveryRecoveryWorker
	occurrenceBootstrap *maintenance.FindingOccurrenceBootstrapWorker
	resourceCloseOnce   sync.Once
}

func NewApplication(config Config) (*Application, error) {
	logger := observability.NewLogger()
	metrics := prometheusmetrics.NewMetrics()
	clock := system.NewClock()
	masker := masking.NewSecretMasker()

	database, databasePool, err := mysql.NewConnection(config.MySQLDSN)
	if err != nil {
		return nil, err
	}
	releaseConstructionResources := true
	defer func() {
		if releaseConstructionResources {
			_ = databasePool.Close()
		}
	}()
	if err := mysql.Migrate(database); err != nil {
		return nil, err
	}

	cache := redisadapter.NewClient(config.RedisAddress, config.RedisPassword, config.RedisDatabase)
	defer func() {
		if releaseConstructionResources {
			_ = cache.Close()
		}
	}()
	redisOptions := asynq.RedisClientOpt{
		Addr:     config.RedisAddress,
		Password: config.RedisPassword,
		DB:       config.RedisDatabase,
	}
	inspector := asynq.NewInspector(redisOptions)
	queueClient := queueadapter.NewClient(asynq.NewClient(redisOptions), inspector)
	defer func() {
		if releaseConstructionResources {
			_ = queueClient.Close()
		}
	}()
	defer func() {
		if releaseConstructionResources {
			_ = inspector.Close()
		}
	}()

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
	progressCheckPublisher := githubadapter.NewProgressCheckPublisher(clients)
	permissions := githubadapter.NewPermissionChecker(clients)
	tools := githubadapter.NewToolExecutorFactory(contents, masker, 16000)
	renderer := markdown.NewRenderer(botName)

	settingSource := settings.NewSettingSource(
		settings.NewConfigLoader(contents, logger),
		settings.NewInstructionCollector(contents, masker, logger),
	)
	reviews := mysql.NewReviewRepository(database)
	reviewRuns := mysql.NewReviewRunStore(database)
	progressChecks := progresscheck.New(progresscheck.Dependencies{
		Checks: progressCheckPublisher,
		Runs:   reviewRuns,
		Logger: logger,
	})
	reviewPublicationStore := mysql.NewReviewPublicationStore(database)
	reviewExecutionStore := mysql.NewReviewExecutionStore(database)
	reviewVerificationStore := mysql.NewReviewVerificationStore(database)
	reviewRetentionRepository := mysql.NewReviewRetentionRepository(database)
	reviewPublicationCandidates := mysql.NewReviewPublicationCandidateStore(database)
	publicationInvalidations := mysql.NewPublicationInvalidationStore(database)
	summaryPublicationRepository := mysql.NewSummaryPublicationRepository(database)
	replyPublicationRepository := mysql.NewReplyPublicationRepository(database)
	pullRequestState := mysql.NewPullRequestStateRepository(database)
	findings := mysql.NewFindingRepository(database)
	occurrenceBootstrapRepository := mysql.NewFindingOccurrenceBootstrapRepository(database)
	installations := mysql.NewInstallationRepository(database)
	usageRepository := mysql.NewUsageRepository(database)
	commands := mysql.NewCommandRepository(database)
	webhookInboxRepository := mysql.NewWebhookInboxRepository(database)
	cooldown := redisadapter.NewCooldown(cache)
	reviewQueue := reviewenqueue.New(reviewenqueue.Dependencies{
		Queue:     queueClient,
		Source:    pullRequests,
		Settings:  settingSource,
		Runs:      reviewRuns,
		Publisher: publisher,
		Renderer:  renderer,
		Checks:    progressChecks,
		Clock:     clock,
		Logger:    logger,
	})

	completer := chain.New(buildProviders(config), cooldown, usageRepository, metrics, clock, logger, config.ProviderCooldown)
	parser := parsing.ResultParser{}

	reviewUseCase := reviewpullrequest.New(reviewpullrequest.Dependencies{
		Source:        pullRequests,
		Settings:      settingSource,
		Masker:        masker,
		Completer:     completer,
		Tools:         tools,
		Publisher:     publisher,
		Reactions:     reactions,
		Renderer:      renderer,
		Reviews:       reviews,
		Runs:          reviewRuns,
		Execution:     reviewExecutionStore,
		Verification:  reviewVerificationStore,
		Publications:  reviewPublicationStore,
		Findings:      findings,
		State:         pullRequestState,
		Progress:      reviewQueue,
		Checks:        progressChecks,
		Clock:         clock,
		Parser:        parser,
		Logger:        logger,
		Retention:     config.ReviewRetention,
		LLMMaxCalls:   config.LLMMaxCalls,
		ReviewTimeout: config.ReviewTimeout,
	})
	reviewRetention := maintenance.NewReviewRetentionWorker(reviewRetentionRepository, usageRepository, clock, logger, config.ReviewRetention)
	reviewPublication := maintenance.NewReviewPublicationWorker(reviewRuns, reviewPublicationStore, reviewPublicationCandidates, publicationInvalidations, pullRequests, publisher, progressChecks, reviews, clock, logger, config.ReviewRetention)
	progressComment := maintenance.NewProgressCommentWorker(reviewRuns, publisher, renderer, progressChecks, clock, logger)
	occurrenceBootstrap := maintenance.NewFindingOccurrenceBootstrapWorker(occurrenceBootstrapRepository, logger)
	summaryUseCase := summarizepullrequest.New(summarizepullrequest.Dependencies{
		Source:       pullRequests,
		Settings:     settingSource,
		Masker:       masker,
		Completer:    completer,
		Publisher:    publisher,
		Renderer:     renderer,
		Reviews:      reviews,
		Publications: summaryPublicationRepository,
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
		Publications: replyPublicationRepository,
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
		Queue:       reviewQueue,
		Commands:    commands,
		Clock:       clock,
		Logger:      logger,
	})

	router := webhook.NewEventRouter(reviewQueue, commandUseCase, inboundcommand.NewParser(botName), installations)
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
		Queue:         reviewQueue,
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
	mux.Handle("/healthz", httpapi.NewHealthHandler(health.NewChecker(database, cache)))
	mux.Handle("/metrics", httpapi.NewMetricsHandler(metrics.Handler(), config.MetricsToken))
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
		RetryDelayFunc: asynq.RetryDelayFunc(func(attempt int, cause error, task *asynq.Task) time.Duration {
			return retryDelayForTask(attempt, cause, time.Now(), task)
		}),
		Logger: newQueueLogger(logger),
	})

	application := &Application{
		logger:              logger,
		database:            databasePool,
		httpServer:          &http.Server{Addr: config.HTTPAddress, Handler: handler, ReadHeaderTimeout: 10 * time.Second},
		asynqServer:         asynqServer,
		asynqMux:            asynqMux,
		queueClient:         queueClient,
		inspector:           inspector,
		cache:               cache,
		reviewRetention:     reviewRetention,
		reviewPublication:   reviewPublication,
		progressComment:     progressComment,
		webhookInbox:        webhookInbox,
		webhookRecovery:     webhookRecovery,
		occurrenceBootstrap: occurrenceBootstrap,
	}
	releaseConstructionResources = false
	return application, nil
}
