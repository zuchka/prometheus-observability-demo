package loadgen

import (
	"context"
	"encoding/json"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestSequenceIncludesExpectedPhases(t *testing.T) {
	steps, err := Sequence("loop")
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) < 6 {
		t.Fatalf("expected several steps, got %d", len(steps))
	}

	seen := map[string]bool{}
	for _, step := range steps {
		seen[step.Phase] = true
		if step.RPS <= 0 {
			t.Fatalf("step %q has invalid RPS %.2f", step.Name, step.RPS)
		}
		if step.Duration <= 0 {
			t.Fatalf("step %q has invalid duration %s", step.Name, step.Duration)
		}
		if len(step.Endpoints) == 0 {
			t.Fatalf("step %q has no endpoints", step.Name)
		}
	}
	for _, phase := range []string{PhaseBaseline, PhaseBurst, PhaseErrorStorm, PhaseLatencySpike, PhaseCPUIOPulse, PhaseRecovery} {
		if !seen[phase] {
			t.Fatalf("sequence missing phase %s", phase)
		}
	}
}

func TestRandomizedLoopSequenceKeepsAllChaosPhases(t *testing.T) {
	base := DefaultSequence()
	randomized := randomizedLoopSequence(base, rand.New(rand.NewSource(7)))
	if len(randomized) != len(base) {
		t.Fatalf("expected %d randomized steps, got %d", len(base), len(randomized))
	}
	if randomized[0].Phase != PhaseBaseline {
		t.Fatalf("expected randomized sequence to start with baseline, got %s", randomized[0].Phase)
	}

	seen := map[string]int{}
	for _, step := range randomized {
		seen[step.Phase]++
		if step.RPS <= 0 {
			t.Fatalf("step %q has invalid RPS %.2f", step.Name, step.RPS)
		}
		if step.Duration <= 0 {
			t.Fatalf("step %q has invalid duration %s", step.Name, step.Duration)
		}
	}

	for _, phase := range []string{PhaseBurst, PhaseErrorStorm, PhaseLatencySpike, PhaseCPUIOPulse} {
		if seen[phase] != 1 {
			t.Fatalf("expected exactly one %s phase, saw %d", phase, seen[phase])
		}
	}
	if seen[PhaseRecovery] != 4 {
		t.Fatalf("expected four recovery phases, saw %d", seen[PhaseRecovery])
	}
}

func TestRandomizedLoopSequenceChangesChaosOrderAndDurations(t *testing.T) {
	base := DefaultSequence()
	randomized := randomizedLoopSequence(base, rand.New(rand.NewSource(3)))

	baseChaos := chaosPhases(base)
	randomizedChaos := chaosPhases(randomized)
	if equalStrings(baseChaos, randomizedChaos) {
		t.Fatalf("expected randomized chaos order to differ from default, got %v", randomizedChaos)
	}

	changedDuration := false
	for i := range base {
		if randomized[i].Duration != base[i].Duration {
			changedDuration = true
			break
		}
	}
	if !changedDuration {
		t.Fatal("expected at least one randomized duration to differ from default")
	}
}

func TestSetChaosSendsTokenAndPayload(t *testing.T) {
	var gotToken string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chaos" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		gotToken = r.Header.Get("X-Demo-Admin-Token")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	gen := New(Config{Target: server.URL, AdminToken: "secret"})
	if err := gen.SetChaos(context.Background(), 2, PhaseLatencySpike); err != nil {
		t.Fatal(err)
	}
	if gotToken != "secret" {
		t.Fatalf("expected token secret, got %q", gotToken)
	}
	if gotBody["mode"].(float64) != 2 || gotBody["phase"].(string) != PhaseLatencySpike {
		t.Fatalf("unexpected body: %#v", gotBody)
	}
}

func chaosPhases(steps []ProfileStep) []string {
	phases := []string{}
	for _, step := range steps {
		switch step.Phase {
		case PhaseBurst, PhaseErrorStorm, PhaseLatencySpike, PhaseCPUIOPulse:
			phases = append(phases, step.Phase)
		}
	}
	return phases
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func TestRunStepGeneratesRequests(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/chaos":
			w.WriteHeader(http.StatusOK)
		case "/api/fast":
			hits.Add(1)
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	gen := New(Config{Target: server.URL, MaxConcurrent: 2})
	step := ProfileStep{
		Name:      "test",
		Phase:     PhaseBaseline,
		ChaosMode: 0,
		RPS:       30,
		Duration:  150 * time.Millisecond,
		Endpoints: []Endpoint{{Method: "GET", Path: "/api/fast", Weight: 1}},
	}
	stats, err := gen.RunStep(context.Background(), step, nil)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	if stats.Requests == 0 || hits.Load() == 0 {
		t.Fatalf("expected generated requests, stats=%+v hits=%d", stats, hits.Load())
	}
}
