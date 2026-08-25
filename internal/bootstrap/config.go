package bootstrap

import (
	"errors"
	"fmt"
	"os"
	"sort"
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
	LLMMaxCalls       int
	DashboardUsername string
	DashboardPassword string
	DashboardSecret   string
	TemplateDirectory string
	StaticDirectory   string
	MetricsToken      string
	DeliveryRetention time.Duration
	ReviewRetention   time.Duration
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
		WorkerConcurrency: number("SANDRONE_WORKER_CONCURRENCY", 2),
		ProviderCooldown:  duration("SANDRONE_PROVIDER_COOLDOWN", 15*time.Minute),
		RequestTimeout:    requestTimeout(),
		LLMMaxCalls:       boundedNumber("SANDRONE_LLM_MAX_CALLS_PER_OPERATION", 24, 1, 24),
		DashboardUsername: os.Getenv("DASHBOARD_USERNAME"),
		DashboardPassword: os.Getenv("DASHBOARD_PASSWORD"),
		DashboardSecret:   os.Getenv("DASHBOARD_SESSION_SECRET"),
		TemplateDirectory: env("SANDRONE_TEMPLATE_DIR", "web/template"),
		StaticDirectory:   env("SANDRONE_STATIC_DIR", "web/static"),
		MetricsToken:      os.Getenv("SANDRONE_METRICS_TOKEN"),
		DeliveryRetention: deliveryRetention(),
		ReviewRetention:   reviewRetention(),
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

	privateCodeProviders, err := privateCodeProviderAllowlist()
	if err != nil {
		return Config{}, err
	}
	config.Providers, err = providerConfigs(privateCodeProviders)
	if err != nil {
		return Config{}, err
	}
	config.ProviderOrder = providerOrder(config.Providers)
	if len(config.ProviderOrder) == 0 {
		return Config{}, errors.New("사용 가능한 LLM 프로바이더가 하나도 설정되지 않았습니다")
	}
	return config, nil
}

func reviewRetention() time.Duration {
	const minimum = 120 * 24 * time.Hour
	const maximum = 180 * 24 * time.Hour
	configured := duration("SANDRONE_REVIEW_RETENTION", maximum)
	if configured < minimum {
		return minimum
	}
	if configured > maximum {
		return maximum
	}
	return configured
}

func deliveryRetention() time.Duration {
	const minimum = 96 * time.Hour
	configured := duration("SANDRONE_DELIVERY_RETENTION", minimum)
	if configured < minimum {
		return minimum
	}
	return configured
}

func requestTimeout() time.Duration {
	const maximum = 5 * time.Minute
	configured := duration("SANDRONE_REQUEST_TIMEOUT", maximum)
	if configured > maximum {
		return maximum
	}
	return configured
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

func providerConfigs(privateCodeProviders map[string]struct{}) (map[string]ProviderConfig, error) {
	catalog := provider.NewCatalog()
	environment := map[string]string{}
	missing := map[string]struct{}{}
	for _, name := range catalog.Order() {
		descriptor, known := catalog.Descriptor(name)
		if !known {
			continue
		}
		environmentNames := []string{descriptor.APIKeyEnv, descriptor.BaseURLEnv, descriptor.ModelEnv, descriptor.AccountIDEnv}
		for _, environmentName := range environmentNames {
			if environmentName == "" {
				continue
			}
			if _, loaded := environment[environmentName]; loaded {
				continue
			}
			environment[environmentName] = strings.TrimSpace(os.Getenv(environmentName))
			if environment[environmentName] == "" && !descriptor.Optional {
				missing[environmentName] = struct{}{}
			}
		}
		if descriptor.Optional {
			configured := false
			for _, environmentName := range environmentNames {
				configured = configured || environmentName != "" && environment[environmentName] != ""
			}
			if configured {
				for _, environmentName := range environmentNames {
					if environmentName != "" && environment[environmentName] == "" {
						missing[environmentName] = struct{}{}
					}
				}
			}
		}
	}
	if len(missing) > 0 {
		missingNames := make([]string, 0, len(missing))
		for name := range missing {
			missingNames = append(missingNames, name)
		}
		sort.Strings(missingNames)
		return nil, fmt.Errorf("필수 LLM 환경 변수가 누락되었습니다: %s", strings.Join(missingNames, ", "))
	}

	configured := map[string]ProviderConfig{}
	for _, name := range catalog.Order() {
		descriptor, known := catalog.Descriptor(name)
		if !known {
			continue
		}
		_, privateCodeAllowed := privateCodeProviders[name]
		model := descriptor.Model
		if descriptor.ModelEnv != "" {
			model = environment[descriptor.ModelEnv]
		}
		baseURL := descriptor.ResolvedBaseURL(environment[descriptor.AccountIDEnv])
		if descriptor.BaseURLEnv != "" {
			baseURL = environment[descriptor.BaseURLEnv]
		}
		candidate := ProviderConfig{
			Name:               name,
			Model:              model,
			APIKey:             environment[descriptor.APIKeyEnv],
			BaseURL:            baseURL,
			PrivateCodeAllowed: privateCodeAllowed,
		}
		if candidate.Enabled() {
			configured[name] = candidate
		}
	}
	return configured, nil
}

func privateCodeProviderAllowlist() (map[string]struct{}, error) {
	catalog := provider.NewCatalog()
	known := map[string]struct{}{}
	for _, name := range catalog.Order() {
		known[name] = struct{}{}
	}
	allowed := map[string]struct{}{}
	for _, raw := range strings.Split(os.Getenv("SANDRONE_PRIVATE_CODE_PROVIDERS"), ",") {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		if _, exists := known[name]; !exists {
			return nil, fmt.Errorf("SANDRONE_PRIVATE_CODE_PROVIDERS에 알 수 없는 프로바이더가 있습니다: %s", name)
		}
		allowed[name] = struct{}{}
	}
	return allowed, nil
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

func boundedNumber(key string, fallback int, minimum int, maximum int) int {
	value := number(key, fallback)
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
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
