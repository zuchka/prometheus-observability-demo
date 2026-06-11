package loadgen

import (
	"fmt"
	"math/rand"
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

func isDefaultLoopSequence(steps []ProfileStep) bool {
	defaultSteps := DefaultSequence()
	if len(steps) != len(defaultSteps) {
		return false
	}
	for i := range steps {
		if steps[i].Phase != defaultSteps[i].Phase || steps[i].ChaosMode != defaultSteps[i].ChaosMode {
			return false
		}
	}
	return true
}

func randomizedLoopSequence(steps []ProfileStep, rng *rand.Rand) []ProfileStep {
	if !isDefaultLoopSequence(steps) {
		return cloneSteps(steps)
	}

	byPhase := make(map[string]ProfileStep, len(steps))
	for _, step := range steps {
		if _, ok := byPhase[step.Phase]; !ok {
			byPhase[step.Phase] = step
		}
	}

	chaos := []ProfileStep{
		byPhase[PhaseBurst],
		byPhase[PhaseErrorStorm],
		byPhase[PhaseLatencySpike],
		byPhase[PhaseCPUIOPulse],
	}
	rng.Shuffle(len(chaos), func(i, j int) {
		chaos[i], chaos[j] = chaos[j], chaos[i]
	})

	sequence := []ProfileStep{
		jitterStep(byPhase[PhaseBaseline], rng, 0.65, 1.55, 0.85, 1.20),
	}
	for _, step := range chaos {
		sequence = append(sequence, jitterStep(step, rng, 0.65, 1.55, 0.85, 1.25))
		sequence = append(sequence, jitterStep(byPhase[PhaseRecovery], rng, 0.45, 2.10, 0.75, 1.35))
	}
	return sequence
}

func cloneSteps(steps []ProfileStep) []ProfileStep {
	cloned := make([]ProfileStep, len(steps))
	copy(cloned, steps)
	return cloned
}

func jitterStep(step ProfileStep, rng *rand.Rand, minDurationFactor, maxDurationFactor, minRPSFactor, maxRPSFactor float64) ProfileStep {
	step.Duration = scaleDuration(step.Duration, rng, minDurationFactor, maxDurationFactor)
	step.RPS = scaleFloat(step.RPS, rng, minRPSFactor, maxRPSFactor)
	return step
}

func scaleDuration(value time.Duration, rng *rand.Rand, minFactor, maxFactor float64) time.Duration {
	scaled := time.Duration(float64(value) * scaleFloat(1, rng, minFactor, maxFactor))
	if scaled < 10*time.Second {
		return 10 * time.Second
	}
	return scaled.Round(time.Second)
}

func scaleFloat(value float64, rng *rand.Rand, minFactor, maxFactor float64) float64 {
	if maxFactor <= minFactor {
		return value * minFactor
	}
	return value * (minFactor + rng.Float64()*(maxFactor-minFactor))
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
