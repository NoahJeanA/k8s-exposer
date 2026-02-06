package api

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Service metrics
	servicesTotal = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "k8s_exposer_services_total",
		Help: "Total number of exposed services",
	})

	portsTotal = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "k8s_exposer_ports_total",
		Help: "Total number of exposed ports",
	})

	// Request metrics
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "k8s_exposer_http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "k8s_exposer_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	// Reconciliation metrics
	reconciliationsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "k8s_exposer_reconciliations_total",
		Help: "Total number of reconciliation runs",
	})

	reconciliationErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "k8s_exposer_reconciliation_errors_total",
		Help: "Total number of reconciliation errors",
	})

	lastReconciliationTime = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "k8s_exposer_last_reconciliation_timestamp_seconds",
		Help: "Unix timestamp of last reconciliation",
	})
)
