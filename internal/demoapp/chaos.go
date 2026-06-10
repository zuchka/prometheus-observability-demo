package demoapp

import (
	"errors"
	"net"
	"strings"
	"sync"
	"sync/atomic"
)

const (
	PhaseManual       = "manual"
	PhaseBaseline     = "baseline"
	PhaseBurst        = "burst"
	PhaseErrorStorm   = "error_storm"
	PhaseLatencySpike = "latency_spike"
	PhaseCPUIOPulse   = "cpu_io_pulse"
	PhaseRecovery     = "recovery"
)

var allowedPhases = []string{
	PhaseManual,
	PhaseBaseline,
	PhaseBurst,
	PhaseErrorStorm,
	PhaseLatencySpike,
	PhaseCPUIOPulse,
	PhaseRecovery,
}

type chaosState struct {
	mode  atomic.Int32
	mu    sync.Mutex
	phase string
}

func AllowedPhases() []string {
	phases := make([]string, len(allowedPhases))
	copy(phases, allowedPhases)
	return phases
}

func validPhase(phase string) bool {
	for _, allowed := range allowedPhases {
		if phase == allowed {
			return true
		}
	}
	return false
}

func normalizePhase(phase string) (string, error) {
	phase = strings.TrimSpace(phase)
	if phase == "" {
		return PhaseManual, nil
	}
	if !validPhase(phase) {
		return "", errors.New("unknown phase")
	}
	return phase, nil
}

func isLoopbackRemote(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = strings.Trim(remoteAddr, "[]")
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
