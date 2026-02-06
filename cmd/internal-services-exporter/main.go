package main

import (
	"log"
	"net/http"
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type InternalService struct {
	Domain    string
	Service   string
	Namespace string
	Port      string
	TargetURL string
}

var internalServices = []InternalService{
	{
		Domain:    "grafana.internal.neverup.at",
		Service:   "kube-prometheus-stack-grafana",
		Namespace: "monitoring",
		Port:      "80",
		TargetURL: "http://kube-prometheus-stack-grafana.monitoring.svc.cluster.local:80",
	},
	{
		Domain:    "prometheus.internal.neverup.at",
		Service:   "kube-prometheus-stack-prometheus",
		Namespace: "monitoring",
		Port:      "9090",
		TargetURL: "http://kube-prometheus-stack-prometheus.monitoring.svc.cluster.local:9090",
	},
	{
		Domain:    "authentik.internal.neverup.at",
		Service:   "authentik-server",
		Namespace: "authentik",
		Port:      "80",
		TargetURL: "http://authentik-server.authentik.svc.cluster.local:80",
	},
	{
		Domain:    "loki.internal.neverup.at",
		Service:   "loki",
		Namespace: "monitoring",
		Port:      "3100",
		TargetURL: "http://loki.monitoring.svc.cluster.local:3100",
	},
	{
		Domain:    "wiki.internal.neverup.at",
		Service:   "mkdocs",
		Namespace: "docs",
		Port:      "80",
		TargetURL: "http://mkdocs.docs.svc.cluster.local:80",
	},
	{
		Domain:    "git.internal.neverup.at",
		Service:   "forgejo-http",
		Namespace: "forgejo",
		Port:      "3000",
		TargetURL: "http://forgejo-http.forgejo.svc.cluster.local:3000",
	},
}

var (
	internalServiceInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "internal_service_info",
			Help: "Information about internal services (*.internal.neverup.at)",
		},
		[]string{"domain", "service", "namespace", "port", "target_url"},
	)
)

func init() {
	prometheus.MustRegister(internalServiceInfo)

	// Set all services to 1 (exists)
	for _, svc := range internalServices {
		internalServiceInfo.WithLabelValues(
			svc.Domain,
			svc.Service,
			svc.Namespace,
			svc.Port,
			svc.TargetURL,
		).Set(1)
	}
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "9092"
	}

	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
<head><title>Internal Services Exporter</title></head>
<body>
<h1>Internal Services Exporter</h1>
<p><a href="/metrics">Metrics</a></p>
</body>
</html>`))
	})

	log.Printf("Starting internal services exporter on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
