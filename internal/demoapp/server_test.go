package demoapp

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthReadyAndMetrics(t *testing.T) {
	server := newTestServer(t, "")
	defer server.Close()

	for _, path := range []string{"/healthz", "/readyz"} {
		resp, err := http.Get(server.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s returned %d", path, resp.StatusCode)
		}
	}

	resp, err := http.Get(server.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "demo_chaos_mode") {
		t.Fatalf("metrics did not include demo_chaos_mode:\n%s", body)
	}
}

func TestInstrumentedFastEndpoint(t *testing.T) {
	server := newTestServer(t, "")
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/fast")
	if err != nil {
		t.Fatalf("GET /api/fast: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/fast returned %d", resp.StatusCode)
	}

	resp, err = http.Get(server.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `demo_http_requests_total{method="GET",route="/api/fast",status="200"} 1`) {
		t.Fatalf("metrics did not include fast request counter:\n%s", body)
	}
}

func TestChaosRequiresTokenWhenConfigured(t *testing.T) {
	server := newTestServer(t, "secret-token")
	defer server.Close()

	body := bytes.NewBufferString(`{"mode":1,"phase":"error_storm"}`)
	resp, err := http.Post(server.URL+"/chaos", "application/json", body)
	if err != nil {
		t.Fatalf("POST /chaos without token: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("POST /chaos without token returned %d", resp.StatusCode)
	}

	req, err := http.NewRequest(http.MethodPost, server.URL+"/chaos", bytes.NewBufferString(`{"mode":1,"phase":"error_storm"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Demo-Admin-Token", "secret-token")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /chaos with token: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /chaos with token returned %d", resp.StatusCode)
	}

	req, err = http.NewRequest(http.MethodGet, server.URL+"/chaos", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer secret-token")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /chaos: %v", err)
	}
	defer resp.Body.Close()
	got, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(got), `"mode":1`) || !strings.Contains(string(got), `"phase":"error_storm"`) {
		t.Fatalf("unexpected chaos response: %s", got)
	}
}

func TestWorkloadEndpointsAreBounded(t *testing.T) {
	server := newTestServer(t, "")
	defer server.Close()

	paths := []string{
		"/api/cpu?ms=10",
		"/api/io?kb=64",
		"/api/memory?mb=1&hold_ms=10",
	}
	for _, path := range paths {
		resp, err := http.Get(server.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s returned %d", path, resp.StatusCode)
		}
	}

	resp, err := http.Get(server.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	for _, metric := range []string{
		"demo_cpu_work_seconds_total",
		"demo_io_work_bytes_total",
		"demo_memory_work_bytes_total",
	} {
		if !strings.Contains(string(body), metric) {
			t.Fatalf("metrics did not include %s:\n%s", metric, body)
		}
	}
}

func newTestServer(t *testing.T, token string) *httptest.Server {
	t.Helper()
	app, err := New(Config{
		Addr:           "127.0.0.1:0",
		AdminToken:     token,
		TempDir:        t.TempDir(),
		MaxMemoryBytes: 8 * 1024 * 1024,
	})
	if err != nil {
		t.Fatalf("New app: %v", err)
	}
	return httptest.NewServer(app.Handler())
}
