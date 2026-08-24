package httpapi

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

type MetricsHandler struct {
	inner http.Handler
	token string
}

func NewMetricsHandler(inner http.Handler, token string) *MetricsHandler {
	return &MetricsHandler{inner: inner, token: token}
}

func (h *MetricsHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if h.token != "" && !h.authorized(request) {
		http.Error(writer, "인증이 필요하다", http.StatusUnauthorized)
		return
	}
	h.inner.ServeHTTP(writer, request)
}

func (h *MetricsHandler) authorized(request *http.Request) bool {
	presented := strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
	if presented == "" {
		presented = request.URL.Query().Get("token")
	}
	return subtle.ConstantTimeCompare([]byte(presented), []byte(h.token)) == 1
}
