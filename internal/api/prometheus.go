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
)
