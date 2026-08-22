package observability

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	registry         *prometheus.Registry
	providerRequests *prometheus.CounterVec
	reviewTotal      *prometheus.CounterVec
	reviewDuration   *prometheus.HistogramVec
	webhookEvents    *prometheus.CounterVec
}

func NewMetrics() *Metrics {
	registry := prometheus.NewRegistry()
	metrics := &Metrics{
		registry: registry,
		providerRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "sandrone_provider_requests_total",
			Help: "LLM 프로바이더 호출 결과별 횟수",
		}, []string{"provider", "outcome"}),
		reviewTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "sandrone_reviews_total",
			Help: "리뷰 작업 처리 결과별 횟수",
		}, []string{"kind", "outcome"}),
		reviewDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "sandrone_review_duration_seconds",
			Help:    "리뷰 작업 처리 시간",
			Buckets: []float64{1, 5, 10, 30, 60, 120, 300, 600},
		}, []string{"kind"}),
		webhookEvents: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "sandrone_webhook_events_total",
			Help: "수신한 웹훅 이벤트 횟수",
		}, []string{"event", "action"}),
	}
	registry.MustRegister(metrics.providerRequests, metrics.reviewTotal, metrics.reviewDuration, metrics.webhookEvents)
	return metrics
}

func (m *Metrics) ObserveProvider(provider string, outcome string) {
	m.providerRequests.WithLabelValues(provider, outcome).Inc()
}

func (m *Metrics) ObserveJob(kind string, outcome string, elapsed time.Duration) {
	m.reviewTotal.WithLabelValues(kind, outcome).Inc()
	m.reviewDuration.WithLabelValues(kind).Observe(elapsed.Seconds())
}

func (m *Metrics) ObserveWebhook(event string, action string) {
	m.webhookEvents.WithLabelValues(event, action).Inc()
}

func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}
