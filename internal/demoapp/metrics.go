package demoapp

import "github.com/prometheus/client_golang/prometheus"

type Metrics struct {
	RequestsTotal       *prometheus.CounterVec
	RequestDuration     *prometheus.HistogramVec
	RequestsInFlight    prometheus.Gauge
	ChaosMode           prometheus.Gauge
	DependencyDuration  *prometheus.HistogramVec
	CPUWorkSeconds      prometheus.Counter
	CPUWorkIterations   prometheus.Counter
	IOWorkBytes         prometheus.Counter
	MemoryWorkBytes     prometheus.Counter
	WorkloadPhase       *prometheus.GaugeVec
	WorkloadEventsTotal *prometheus.CounterVec
}

func newMetrics(reg *prometheus.Registry) (*Metrics, error) {
	metrics := &Metrics{
		RequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "demo_http_requests_total",
				Help: "Total demo app HTTP requests by method, route, and status code.",
			},
			[]string{"method", "route", "status"},
		),
		RequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "demo_http_request_duration_seconds",
				Help:    "Demo app HTTP request latency in seconds.",
				Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
			},
			[]string{"method", "route"},
		),
		RequestsInFlight: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "demo_http_requests_in_flight",
				Help: "Current number of demo app HTTP requests being handled.",
			},
		),
		ChaosMode: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "demo_chaos_mode",
				Help: "Current chaos mode. 0 normal, 1 error storm, 2 latency spike, 3 degraded mix.",
			},
		),
		DependencyDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "demo_dependency_duration_seconds",
				Help:    "Synthetic dependency latency in seconds.",
				Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
			},
			[]string{"dependency"},
		),
		CPUWorkSeconds: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "demo_cpu_work_seconds_total",
				Help: "Total seconds spent doing synthetic CPU work.",
			},
		),
		CPUWorkIterations: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "demo_cpu_work_iterations_total",
				Help: "Total synthetic CPU loop iterations.",
			},
		),
		IOWorkBytes: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "demo_io_work_bytes_total",
				Help: "Total bytes written during synthetic disk I/O work.",
			},
		),
		MemoryWorkBytes: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "demo_memory_work_bytes_total",
				Help: "Total bytes allocated during synthetic memory work.",
			},
		),
		WorkloadPhase: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "demo_workload_phase",
				Help: "Current traffic generator phase. The active phase has value 1.",
			},
			[]string{"phase"},
		),
		WorkloadEventsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "demo_workload_events_total",
				Help: "Traffic generator phase change events by phase.",
			},
			[]string{"phase"},
		),
	}

	collectors := []prometheus.Collector{
		metrics.RequestsTotal,
		metrics.RequestDuration,
		metrics.RequestsInFlight,
		metrics.ChaosMode,
		metrics.DependencyDuration,
		metrics.CPUWorkSeconds,
		metrics.CPUWorkIterations,
		metrics.IOWorkBytes,
		metrics.MemoryWorkBytes,
		metrics.WorkloadPhase,
		metrics.WorkloadEventsTotal,
	}
	for _, collector := range collectors {
		if err := reg.Register(collector); err != nil {
			return nil, err
		}
	}

	for _, phase := range AllowedPhases() {
		metrics.WorkloadPhase.WithLabelValues(phase).Set(0)
	}
	metrics.WorkloadPhase.WithLabelValues(PhaseManual).Set(1)
	return metrics, nil
}
