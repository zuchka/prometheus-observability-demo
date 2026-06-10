package demoapp

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type App struct {
	cfg      Config
	registry *prometheus.Registry
	metrics  *Metrics
	chaos    chaosState
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

type chaosRequest struct {
	Mode  int32  `json:"mode"`
	Phase string `json:"phase"`
}

type chaosResponse struct {
	Mode  int32  `json:"mode"`
	Phase string `json:"phase"`
}

var cpuSink atomic.Uint64

func New(cfg Config) (*App, error) {
	if cfg.Addr == "" {
		cfg.Addr = "127.0.0.1:8080"
	}
	if cfg.TempDir == "" {
		cfg.TempDir = filepath.Join(os.TempDir(), "prometheus-observability-demo")
	}
	if cfg.MaxMemoryBytes <= 0 {
		cfg.MaxMemoryBytes = 48 * 1024 * 1024
	}
	if err := os.MkdirAll(cfg.TempDir, 0o755); err != nil {
		return nil, err
	}

	registry := prometheus.NewRegistry()
	metrics, err := newMetrics(registry)
	if err != nil {
		return nil, err
	}

	app := &App{
		cfg:      cfg,
		registry: registry,
		metrics:  metrics,
	}
	app.chaos.phase = PhaseManual
	return app, nil
}

func (a *App) Registry() *prometheus.Registry {
	return a.registry
}

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.handleHealth)
	mux.HandleFunc("GET /readyz", a.handleReady)
	mux.Handle("GET /metrics", promhttp.HandlerFor(a.registry, promhttp.HandlerOpts{}))
	mux.HandleFunc("GET /api/fast", a.instrument("/api/fast", a.handleFast))
	mux.HandleFunc("GET /api/slow", a.instrument("/api/slow", a.handleSlow))
	mux.HandleFunc("GET /api/flaky", a.instrument("/api/flaky", a.handleFlaky))
	mux.HandleFunc("GET /api/cpu", a.instrument("/api/cpu", a.handleCPU))
	mux.HandleFunc("GET /api/io", a.instrument("/api/io", a.handleIO))
	mux.HandleFunc("GET /api/memory", a.instrument("/api/memory", a.handleMemory))
	mux.HandleFunc("/chaos", a.handleChaos)
	return mux
}

func (r *statusRecorder) WriteHeader(status int) {
	if r.wrote {
		return
	}
	r.status = status
	r.wrote = true
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(data []byte) (int, error) {
	if !r.wrote {
		r.WriteHeader(http.StatusOK)
	}
	return r.ResponseWriter.Write(data)
}

func (a *App) instrument(route string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a.metrics.RequestsInFlight.Inc()
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			if recovered := recover(); recovered != nil {
				if !recorder.wrote {
					http.Error(recorder, `{"error":"internal server error"}`, http.StatusInternalServerError)
				}
			}
			status := strconv.Itoa(recorder.status)
			a.metrics.RequestsTotal.WithLabelValues(r.Method, route, status).Inc()
			a.metrics.RequestDuration.WithLabelValues(r.Method, route).Observe(time.Since(start).Seconds())
			a.metrics.RequestsInFlight.Dec()
		}()
		h(recorder, r)
	}
}

func (a *App) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *App) handleReady(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (a *App) handleFast(w http.ResponseWriter, _ *http.Request) {
	a.syntheticDependency("cache", 8, 22)
	if a.shouldFail(0.005) {
		writeError(w, http.StatusInternalServerError, "fast path failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"endpoint": "fast",
		"status":   "ok",
		"items":    3,
	})
}

func (a *App) handleSlow(w http.ResponseWriter, _ *http.Request) {
	a.syntheticDependency("database", 180, 520)
	a.syntheticDependency("search", 35, 110)
	if a.shouldFail(0.02) {
		writeError(w, http.StatusInternalServerError, "slow path dependency failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"endpoint": "slow",
		"status":   "ok",
		"rows":     42,
	})
}

func (a *App) handleFlaky(w http.ResponseWriter, _ *http.Request) {
	a.syntheticDependency("payments", 35, 140)
	if a.shouldFail(0.12) {
		writeError(w, http.StatusInternalServerError, "flaky dependency unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"endpoint": "flaky",
		"status":   "ok",
	})
}

func (a *App) handleCPU(w http.ResponseWriter, r *http.Request) {
	defaultMS := 80
	if a.currentMode() == 3 {
		defaultMS = 150
	}
	ms := boundedQueryInt(r, "ms", defaultMS, 10, 350)
	elapsed, iterations := burnCPU(time.Duration(ms) * time.Millisecond)
	a.metrics.CPUWorkSeconds.Add(elapsed.Seconds())
	a.metrics.CPUWorkIterations.Add(float64(iterations))
	writeJSON(w, http.StatusOK, map[string]any{
		"endpoint":       "cpu",
		"requested_ms":   ms,
		"actual_ms":      elapsed.Milliseconds(),
		"iterations":     iterations,
		"accumulator_id": cpuSink.Load(),
	})
}

func (a *App) handleIO(w http.ResponseWriter, r *http.Request) {
	kb := boundedQueryInt(r, "kb", 512, 64, 4096)
	bytesWritten, err := a.writeTemporaryFile(kb * 1024)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.metrics.IOWorkBytes.Add(float64(bytesWritten))
	writeJSON(w, http.StatusOK, map[string]any{
		"endpoint":      "io",
		"bytes_written": bytesWritten,
	})
}

func (a *App) handleMemory(w http.ResponseWriter, r *http.Request) {
	maxMB := int(a.cfg.MaxMemoryBytes / 1024 / 1024)
	mb := boundedQueryInt(r, "mb", 16, 1, maxMB)
	holdMS := boundedQueryInt(r, "hold_ms", 250, 10, 1500)
	size := mb * 1024 * 1024
	buf := make([]byte, size)
	for i := 0; i < len(buf); i += 4096 {
		buf[i] = byte(i)
	}
	time.Sleep(time.Duration(holdMS) * time.Millisecond)
	runtime.KeepAlive(buf)
	a.metrics.MemoryWorkBytes.Add(float64(size))
	writeJSON(w, http.StatusOK, map[string]any{
		"endpoint":        "memory",
		"bytes_allocated": size,
		"held_ms":         holdMS,
	})
}

func (a *App) handleChaos(w http.ResponseWriter, r *http.Request) {
	if !isLoopbackRemote(r.RemoteAddr) {
		writeError(w, http.StatusForbidden, "chaos endpoint is only available from loopback")
		return
	}
	if !a.authorized(r) {
		writeError(w, http.StatusUnauthorized, "missing or invalid admin token")
		return
	}

	switch r.Method {
	case http.MethodGet:
		mode, phase := a.getChaos()
		writeJSON(w, http.StatusOK, chaosResponse{Mode: mode, Phase: phase})
	case http.MethodPost:
		var req chaosRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if req.Mode < 0 || req.Mode > 3 {
			writeError(w, http.StatusBadRequest, "mode must be 0, 1, 2, or 3")
			return
		}
		phase, err := normalizePhase(req.Phase)
		if err != nil {
			writeError(w, http.StatusBadRequest, "phase must be one of: "+strings.Join(AllowedPhases(), ", "))
			return
		}
		a.setChaos(req.Mode, phase)
		writeJSON(w, http.StatusOK, chaosResponse{Mode: req.Mode, Phase: phase})
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (a *App) authorized(r *http.Request) bool {
	if a.cfg.AdminToken == "" {
		return true
	}
	token := r.Header.Get("X-Demo-Admin-Token")
	if token == "" {
		auth := r.Header.Get("Authorization")
		token = strings.TrimPrefix(auth, "Bearer ")
	}
	if token == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(a.cfg.AdminToken)) == 1
}

func (a *App) getChaos() (int32, string) {
	a.chaos.mu.Lock()
	defer a.chaos.mu.Unlock()
	return a.chaos.mode.Load(), a.chaos.phase
}

func (a *App) setChaos(mode int32, phase string) {
	a.chaos.mu.Lock()
	previous := a.chaos.phase
	a.chaos.phase = phase
	a.chaos.mu.Unlock()

	a.chaos.mode.Store(mode)
	a.metrics.ChaosMode.Set(float64(mode))
	if previous != phase {
		a.metrics.WorkloadPhase.WithLabelValues(previous).Set(0)
	}
	a.metrics.WorkloadPhase.WithLabelValues(phase).Set(1)
	a.metrics.WorkloadEventsTotal.WithLabelValues(phase).Inc()
}

func (a *App) currentMode() int32 {
	return a.chaos.mode.Load()
}

func (a *App) shouldFail(baseRate float64) bool {
	rate := baseRate
	switch a.currentMode() {
	case 1:
		rate += 0.35
	case 2:
		rate += 0.03
	case 3:
		rate += 0.25
	}
	if rate > 0.95 {
		rate = 0.95
	}
	return rand.Float64() < rate
}

func (a *App) latencyMultiplier() float64 {
	switch a.currentMode() {
	case 2:
		return 5
	case 3:
		return 2.5
	default:
		return 1
	}
}

func (a *App) syntheticDependency(name string, baseMS, jitterMS int) {
	delayMS := float64(baseMS+rand.Intn(jitterMS+1)) * a.latencyMultiplier()
	delay := time.Duration(delayMS) * time.Millisecond
	start := time.Now()
	time.Sleep(delay)
	a.metrics.DependencyDuration.WithLabelValues(name).Observe(time.Since(start).Seconds())
}

func (a *App) writeTemporaryFile(size int) (int, error) {
	if size <= 0 {
		return 0, errors.New("size must be positive")
	}
	file, err := os.CreateTemp(a.cfg.TempDir, "demo-io-*.bin")
	if err != nil {
		return 0, fmt.Errorf("create temp file: %w", err)
	}
	name := file.Name()
	defer os.Remove(name)
	defer file.Close()

	chunk := make([]byte, 32*1024)
	for i := range chunk {
		chunk[i] = byte(i % 251)
	}

	written := 0
	for written < size {
		next := chunk
		if remaining := size - written; remaining < len(chunk) {
			next = chunk[:remaining]
		}
		n, err := file.Write(next)
		written += n
		if err != nil {
			return written, fmt.Errorf("write temp file: %w", err)
		}
	}
	if err := file.Sync(); err != nil {
		return written, fmt.Errorf("sync temp file: %w", err)
	}
	return written, nil
}

func burnCPU(duration time.Duration) (time.Duration, uint64) {
	start := time.Now()
	deadline := start.Add(duration)
	var x uint64 = 1469598103934665603
	var iterations uint64
	for time.Now().Before(deadline) {
		x ^= iterations + 0x9e3779b97f4a7c15
		x *= 1099511628211
		iterations++
	}
	cpuSink.Add(x)
	return time.Since(start), iterations
}

func boundedQueryInt(r *http.Request, key string, fallback, min, max int) int {
	if max < min {
		max = min
	}
	value := r.URL.Query().Get(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	if parsed < min {
		return min
	}
	if parsed > max {
		return max
	}
	return parsed
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
