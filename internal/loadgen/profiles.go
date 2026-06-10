package loadgen

import (
	"fmt"
	"time"
)

const (
	PhaseBaseline     = "baseline"
	PhaseBurst        = "burst"
	PhaseErrorStorm   = "error_storm"
	PhaseLatencySpike = "latency_spike"
	PhaseCPUIOPulse   = "cpu_io_pulse"
	PhaseRecovery     = "recovery"
)

type Endpoint struct {
	Method string
	Path   string
	Weight int
}

type ProfileStep struct {
	Name      string
	Phase     string
	ChaosMode int32
	RPS       float64
	Duration  time.Duration
	Endpoints []Endpoint
}

func Sequence(name string) ([]ProfileStep, error) {
	switch name {
	case "", "loop", "demo":
		return DefaultSequence(), nil
	case "baseline":
		step := baselineStep()
		step.Duration = 10 * time.Minute
		return []ProfileStep{step}, nil
	case "burst":
		step := burstStep()
		step.Duration = 10 * time.Minute
		return []ProfileStep{step}, nil
	case "error-storm":
		step := errorStormStep()
		step.Duration = 10 * time.Minute
		return []ProfileStep{step}, nil
	case "latency-spike":
		step := latencySpikeStep()
		step.Duration = 10 * time.Minute
		return []ProfileStep{step}, nil
	case "cpu-io-pulse":
		step := cpuIOPulseStep()
		step.Duration = 10 * time.Minute
		return []ProfileStep{step}, nil
	case "recovery":
		step := recoveryStep()
		step.Duration = 10 * time.Minute
		return []ProfileStep{step}, nil
	default:
		return nil, fmt.Errorf("unknown profile %q", name)
	}
}

func DefaultSequence() []ProfileStep {
	return []ProfileStep{
		baselineStep(),
		burstStep(),
		recoveryStep(),
		errorStormStep(),
		recoveryStep(),
		latencySpikeStep(),
		recoveryStep(),
		cpuIOPulseStep(),
		recoveryStep(),
	}
}

func baselineStep() ProfileStep {
	return ProfileStep{
		Name:      "Baseline traffic",
		Phase:     PhaseBaseline,
		ChaosMode: 0,
		RPS:       1.2,
		Duration:  3 * time.Minute,
		Endpoints: defaultEndpoints(),
	}
}

func burstStep() ProfileStep {
	return ProfileStep{
		Name:      "Traffic burst",
		Phase:     PhaseBurst,
		ChaosMode: 0,
		RPS:       8,
		Duration:  90 * time.Second,
		Endpoints: defaultEndpoints(),
	}
}

func errorStormStep() ProfileStep {
	return ProfileStep{
		Name:      "Error storm",
		Phase:     PhaseErrorStorm,
		ChaosMode: 1,
		RPS:       3,
		Duration:  2 * time.Minute,
		Endpoints: defaultEndpoints(),
	}
}

func latencySpikeStep() ProfileStep {
	return ProfileStep{
		Name:      "Latency spike",
		Phase:     PhaseLatencySpike,
		ChaosMode: 2,
		RPS:       3,
		Duration:  2 * time.Minute,
		Endpoints: defaultEndpoints(),
	}
}

func cpuIOPulseStep() ProfileStep {
	return ProfileStep{
		Name:      "CPU and I/O pulse",
		Phase:     PhaseCPUIOPulse,
		ChaosMode: 3,
		RPS:       2,
		Duration:  2 * time.Minute,
		Endpoints: []Endpoint{
			{Method: "GET", Path: "/api/cpu", Weight: 40},
			{Method: "GET", Path: "/api/io", Weight: 30},
			{Method: "GET", Path: "/api/memory", Weight: 15},
			{Method: "GET", Path: "/api/fast", Weight: 15},
		},
	}
}

func recoveryStep() ProfileStep {
	return ProfileStep{
		Name:      "Recovery",
		Phase:     PhaseRecovery,
		ChaosMode: 0,
		RPS:       1,
		Duration:  45 * time.Second,
		Endpoints: defaultEndpoints(),
	}
}

func defaultEndpoints() []Endpoint {
	return []Endpoint{
		{Method: "GET", Path: "/api/fast", Weight: 45},
		{Method: "GET", Path: "/api/slow", Weight: 20},
		{Method: "GET", Path: "/api/flaky", Weight: 15},
		{Method: "GET", Path: "/api/cpu", Weight: 8},
		{Method: "GET", Path: "/api/io", Weight: 7},
		{Method: "GET", Path: "/api/memory", Weight: 5},
	}
}
