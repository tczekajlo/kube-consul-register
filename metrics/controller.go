package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	// PodFailure returns counter for pod_errors_total metric
	PodFailure = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pod_errors_total",
			Help: "Number of failure operation on PODs",
		},
		[]string{"operation"},
	)

	// PodSuccess returns counter for pod_successes_total metric
	PodSuccess = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pod_successes_total",
			Help: "Number of succeed operation on PODs",
		},
		[]string{"operation"},
	)

	// FuncDuration returns summary for controller_function_duration_seconds metric
	FuncDuration = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "controller_function_duration_seconds",
			Help:       "The runtime of an function.",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		},
		[]string{"function"},
	)
)
