package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type HealthHandler struct {
	database *gorm.DB
	cache    *redis.Client
}

func NewHealthHandler(database *gorm.DB, cache *redis.Client) *HealthHandler {
	return &HealthHandler{database: database, cache: cache}
}

func (h *HealthHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	status := map[string]string{"mysql": "ok", "redis": "ok"}
	ready := true

	pool, err := h.database.DB()
	if err != nil || pool.PingContext(request.Context()) != nil {
		status["mysql"] = "unreachable"
		ready = false
	}
	if err := h.cache.Ping(request.Context()).Err(); err != nil {
		status["redis"] = "degraded"
	}

	writer.Header().Set("Content-Type", "application/json")
	if !ready {
		writer.WriteHeader(http.StatusServiceUnavailable)
	}
	_ = json.NewEncoder(writer).Encode(status)
}
