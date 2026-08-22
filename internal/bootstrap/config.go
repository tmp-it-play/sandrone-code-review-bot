package bootstrap

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/it-play/sandrone-code-review-bot/internal/adapter/outbound/llm/provider"
)

type Config struct {
	HTTPAddress       string
	BasePath          string
	AppID             int64
	PrivateKey        []byte
	WebhookSecret     string
	MySQLDSN          string
	RedisAddress      string
	RedisPassword     string
	RedisDatabase     int
	WorkerConcurrency int
	ProviderOrder     []string
	Providers         map[string]ProviderConfig
	ProviderCooldown  time.Duration
	RequestTimeout    time.Duration
	DashboardUsername string
	DashboardPassword string
	DashboardSecret   string
	TemplateDirectory string
	StaticDirectory   string
	MetricsToken      string
	DeliveryRetention time.Duration
}

func LoadConfig() (Config, error) {
	config := Config{
		HTTPAddress:       env("SANDRONE_HTTP_ADDR", ":8080"),
		BasePath:          normalizeBasePath(os.Getenv("SANDRONE_BASE_PATH")),
		WebhookSecret:     os.Getenv("GITHUB_WEBHOOK_SECRET"),
		MySQLDSN:          os.Getenv("MYSQL_DSN"),
		RedisAddress:      env("REDIS_ADDR", "127.0.0.1:6379"),
		RedisPassword:     os.Getenv("REDIS_PASSWORD"),
		RedisDatabase:     number("REDIS_DB", 0),
		WorkerConcurrency: number("SANDRONE_WORKER_CONCURRENCY", 4),
		ProviderCooldown:  duration("SANDRONE_PROVIDER_COOLDOWN", 15*time.Minute),
		RequestTimeout:    duration("SANDRONE_REQUEST_TIMEOUT", 5*time.Minute),
		DashboardUsername: os.Getenv("DASHBOARD_USERNAME"),
		DashboardPassword: os.Getenv("DASHBOARD_PASSWORD"),
		DashboardSecret:   os.Getenv("DASHBOARD_SESSION_SECRET"),
		TemplateDirectory: env("SANDRONE_TEMPLATE_DIR", "web/template"),
		StaticDirectory:   env("SANDRONE_STATIC_DIR", "web/static"),
		MetricsToken:      os.Getenv("SANDRONE_METRICS_TOKEN"),
		DeliveryRetention: duration("SANDRONE_DELIVERY_RETENTION", 24*time.Hour),
	}

	appID, err := strconv.ParseInt(strings.TrimSpace(os.Getenv("GITHUB_APP_ID")), 10, 64)
	if err != nil {
		return Config{}, errors.New("GITHUB_APP_ID가 없거나 숫자가 아닙니다")
	}
	config.AppID = appID

	key, err := privateKey()
	if err != nil {
		return Config{}, err
	}
	config.PrivateKey = key

	if config.WebhookSecret == "" {
		return Config{}, errors.New("GITHUB_WEBHOOK_SECRET이 필요하다")
	}
	if config.MySQLDSN == "" {
		return Config{}, errors.New("MYSQL_DSN이 필요하다")
	}
	if config.DashboardUsername == "" || config.DashboardPassword == "" {
		return Config{}, errors.New("DASHBOARD_USERNAME과 DASHBOARD_PASSWORD가 필요하다")
	}
	if config.DashboardSecret == "" {
		return Config{}, errors.New("DASHBOARD_SESSION_SECRET이 필요하다")
	}

	config.Providers = providerConfigs()
	config.ProviderOrder = providerOrder(config.Providers)
	if len(config.ProviderOrder) == 0 {
		return Config{}, errors.New("사용 가능한 LLM 프로바이더가 하나도 설정되지 않았습니다")
	}
	return config, nil
}

func privateKey() ([]byte, error) {
	if inline := os.Getenv("GITHUB_PRIVATE_KEY"); strings.TrimSpace(inline) != "" {
		return []byte(strings.ReplaceAll(inline, "\\n", "\n")), nil
	}
	path := os.Getenv("GITHUB_PRIVATE_KEY_PATH")
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("GITHUB_PRIVATE_KEY 또는 GITHUB_PRIVATE_KEY_PATH가 필요하다")
	}
	return os.ReadFile(path)
}

func providerConfigs() map[string]ProviderConfig {
	catalog := provider.NewCatalog()
	configured := map[string]ProviderConfig{}
	for _, name := range catalog.Order() {
		candidate := ProviderConfig{Name: name, APIKey: os.Getenv(catalog.APIKeyEnv(name))}
		if candidate.Enabled() {
			configured[name] = candidate
		}
	}
	return configured
}

func providerOrder(configured map[string]ProviderConfig) []string {
	preferred := provider.NewCatalog().Order()
	order := make([]string, 0, len(preferred))
	for _, name := range preferred {
		if _, ok := configured[name]; ok {
			order = append(order, name)
		}
	}
	return order
}

func env(key string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func number(key string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key)))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func duration(key string, fallback time.Duration) time.Duration {
	value, err := time.ParseDuration(strings.TrimSpace(os.Getenv(key)))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func normalizeBasePath(raw string) string {
	trimmed := strings.Trim(strings.TrimSpace(raw), "/")
	if trimmed == "" {
		return ""
	}
	return "/" + trimmed
}
