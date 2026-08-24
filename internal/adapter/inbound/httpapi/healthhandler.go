package httpapi

import (
	"encoding/json"
	"net/http"
)

type HealthHandler struct {
	checker ReadinessChecker
}

func NewHealthHandler(checker ReadinessChecker) *HealthHandler {
	return &HealthHandler{checker: checker}
}

func (h *HealthHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	status, ready := h.checker.Check(request.Context())

	writer.Header().Set("Content-Type", "application/json")
	if !ready {
		writer.WriteHeader(http.StatusServiceUnavailable)
	}
	_ = json.NewEncoder(writer).Encode(status)
}
