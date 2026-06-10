package loadgen

import (
	"context"
	"encoding/json"
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
