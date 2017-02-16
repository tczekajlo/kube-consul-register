package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	// ConsulFailure returns counter for consul_errors_total metric
	ConsulFailure = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "consul_errors_total",
			Help: "Number of Consul errors for HTTP request.",
		},
		[]string{"operation", "consul_address"},
	)

	// ConsulSuccess returns counter for consul_successes_total metric
	ConsulSuccess = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "consul_successes_total",
			Help: "Number of Consul success for HTTP request.",
		},
		[]string{"operation", "consul_address"},
	)
)
