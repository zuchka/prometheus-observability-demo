package loadgen

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Config struct {
	Target        string
	AdminToken    string
	MaxConcurrent int
}

type Generator struct {
	cfg    Config
	client *http.Client
	rng    *rand.Rand
	mu     sync.Mutex
	sem    chan struct{}
}

type StepStats struct {
	Requests int64
	Errors   int64
}

func New(cfg Config) *Generator {
	if cfg.Target == "" {
		cfg.Target = "http://127.0.0.1:8080"
	}
	cfg.Target = strings.TrimRight(cfg.Target, "/")
	if cfg.MaxConcurrent <= 0 {
		cfg.MaxConcurrent = 12
	}
	return &Generator{
		cfg: cfg,
		client: &http.Client{
			Timeout: 8 * time.Second,
		},
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
		sem: make(chan struct{}, cfg.MaxConcurrent),
	}
}

func (g *Generator) RunSequence(ctx context.Context, steps []ProfileStep, loop bool, logf func(string, ...any)) error {
	if len(steps) == 0 {
		return errors.New("sequence must contain at least one step")
	}
	if logf == nil {
		logf = func(string, ...any) {}
	}
	for {
		cycle := steps
		if isDefaultLoopSequence(steps) {
			cycle = g.randomizedLoopSequence(steps)
			logf("starting randomized loop cycle phases=%s", strings.Join(stepPhases(cycle), ","))
		}
		for _, step := range cycle {
			stats, err := g.RunStep(ctx, step, logf)
			if err != nil {
				return err
			}
			logf("completed phase=%s requests=%d errors=%d", step.Phase, stats.Requests, stats.Errors)
		}
		if !loop {
			return nil
		}
	}
}

func (g *Generator) RunStep(ctx context.Context, step ProfileStep, logf func(string, ...any)) (StepStats, error) {
	if step.RPS <= 0 {
		return StepStats{}, fmt.Errorf("step %q has invalid RPS %.2f", step.Name, step.RPS)
	}
	if step.Duration <= 0 {
		return StepStats{}, fmt.Errorf("step %q has invalid duration %s", step.Name, step.Duration)
	}
	if len(step.Endpoints) == 0 {
		return StepStats{}, fmt.Errorf("step %q has no endpoints", step.Name)
	}
	if logf == nil {
		logf = func(string, ...any) {}
	}

	if err := g.SetChaos(ctx, step.ChaosMode, step.Phase); err != nil {
		logf("warning: could not set chaos mode: %v", err)
	}
	logf("starting phase=%s mode=%d rps=%.2f duration=%s", step.Phase, step.ChaosMode, step.RPS, step.Duration)

	var requests atomic.Int64
	var errors atomic.Int64
	interval := time.Duration(float64(time.Second) / step.RPS)
	if interval < 25*time.Millisecond {
		interval = 25 * time.Millisecond
	}

	timer := time.NewTimer(step.Duration)
	requestTimer := time.NewTimer(g.jitteredInterval(interval))
	defer timer.Stop()
	defer requestTimer.Stop()

	for {
		select {
		case <-ctx.Done():
			return StepStats{Requests: requests.Load(), Errors: errors.Load()}, ctx.Err()
		case <-timer.C:
			return StepStats{Requests: requests.Load(), Errors: errors.Load()}, nil
		case <-requestTimer.C:
			endpoint := g.pick(step.Endpoints)
			requests.Add(1)
			go func() {
				if err := g.send(ctx, endpoint); err != nil {
					errors.Add(1)
				}
			}()
			requestTimer.Reset(g.jitteredInterval(interval))
		}
	}
}

func (g *Generator) SetChaos(ctx context.Context, mode int32, phase string) error {
	payload, err := json.Marshal(map[string]any{
		"mode":  mode,
		"phase": phase,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.cfg.Target+"/chaos", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if g.cfg.AdminToken != "" {
		req.Header.Set("X-Demo-Admin-Token", g.cfg.AdminToken)
	}
	resp, err := g.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("set chaos returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func (g *Generator) send(ctx context.Context, endpoint Endpoint) error {
	select {
	case g.sem <- struct{}{}:
		defer func() { <-g.sem }()
	case <-ctx.Done():
		return ctx.Err()
	}

	req, err := http.NewRequestWithContext(ctx, endpoint.Method, g.cfg.Target+endpoint.Path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "demo-load/1.0")
	resp, err := g.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 500 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

func (g *Generator) pick(endpoints []Endpoint) Endpoint {
	total := 0
	for _, endpoint := range endpoints {
		if endpoint.Weight > 0 {
			total += endpoint.Weight
		}
	}
	if total <= 0 {
		return endpoints[0]
	}

	g.mu.Lock()
	n := g.rng.Intn(total)
	g.mu.Unlock()
	for _, endpoint := range endpoints {
		if endpoint.Weight <= 0 {
			continue
		}
		n -= endpoint.Weight
		if n < 0 {
			return endpoint
		}
	}
	return endpoints[0]
}

func (g *Generator) randomizedLoopSequence(steps []ProfileStep) []ProfileStep {
	g.mu.Lock()
	defer g.mu.Unlock()
	return randomizedLoopSequence(steps, g.rng)
}

func (g *Generator) jitteredInterval(base time.Duration) time.Duration {
	g.mu.Lock()
	factor := 0.35 + g.rng.Float64()*1.30
	g.mu.Unlock()

	interval := time.Duration(float64(base) * factor)
	if interval < 25*time.Millisecond {
		return 25 * time.Millisecond
	}
	return interval
}

func stepPhases(steps []ProfileStep) []string {
	phases := make([]string, 0, len(steps))
	for _, step := range steps {
		phases = append(phases, step.Phase)
	}
	return phases
}
