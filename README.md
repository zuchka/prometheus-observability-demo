# Prometheus Observability Demo Toolkit

This project creates useful Prometheus and Grafana demo data on a small 1 vCPU, 512 MB RAM DigitalOcean droplet.

It provides:

- `demo-api`: a private localhost Go HTTP app with Prometheus metrics.
- `demo-load`: a bounded traffic generator that cycles through realistic workload phases.
- systemd units for a droplet that already runs Prometheus and node_exporter.
- a Prometheus scrape snippet.
- an importable Grafana app dashboard.
- runbooks for setup, operation, and troubleshooting.

The intended public surface is dashboards only. The app, `/metrics`, and `/chaos` bind to localhost and should not be exposed directly.

## Architecture

```text
demo-load -> http://127.0.0.1:8080 -> demo-api -> /metrics
                                                ^
                                                |
Prometheus scrapes 127.0.0.1:8080 -------------+

node_exporter continues to expose host metrics on 127.0.0.1:9100 or your existing bind address.
Grafana reads Prometheus and displays both the existing Node Exporter dashboard and the new app dashboard.
```

## What Moves in Dashboards

The existing Node Exporter dashboard should become less flat because the demo app performs bounded CPU, memory, disk I/O, and request handling work.

The new app dashboard shows application behavior that Node Exporter cannot show:

- request rate by route
- status code mix
- error ratio
- p50, p95, and p99 latency
- in-flight requests
- synthetic dependency latency
- workload phase and chaos mode
- CPU, I/O, and memory work counters

## Local Quickstart

Build and test:

```bash
make build
make test
make vet
```

Start the API:

```bash
DEMO_ADMIN_TOKEN=dev-secret ./bin/demo-api
```

In another terminal, verify the app:

```bash
curl -s http://127.0.0.1:8080/healthz
curl -s http://127.0.0.1:8080/readyz
curl -s http://127.0.0.1:8080/metrics | grep '^demo_'
```

Run the generator:

```bash
DEMO_ADMIN_TOKEN=dev-secret ./bin/demo-load -profile=loop
```

Run a single profile instead:

```bash
DEMO_ADMIN_TOKEN=dev-secret ./bin/demo-load -profile=cpu-io-pulse -once -step-duration=30s
```

Manually change chaos mode:

```bash
curl -s \
  -H 'Content-Type: application/json' \
  -H 'X-Demo-Admin-Token: dev-secret' \
  -d '{"mode":2,"phase":"latency_spike"}' \
  http://127.0.0.1:8080/chaos
```

Reset to normal:

```bash
curl -s \
  -H 'Content-Type: application/json' \
  -H 'X-Demo-Admin-Token: dev-secret' \
  -d '{"mode":0,"phase":"recovery"}' \
  http://127.0.0.1:8080/chaos
```

## Traffic Profiles

The default `loop` profile repeats these phases:

- `baseline`: low steady traffic.
- `burst`: short request-rate spike.
- `recovery`: low traffic with normal behavior.
- `error_storm`: higher 5xx rate.
- `latency_spike`: slower synthetic dependencies.
- `cpu_io_pulse`: more CPU, disk I/O, and memory activity.

The generator keeps rates conservative for a 512 MB droplet. To run one phase, use `-profile=baseline`, `-profile=burst`, `-profile=error-storm`, `-profile=latency-spike`, `-profile=cpu-io-pulse`, or `-profile=recovery`.

## Deployment

Use [docs/RUNBOOK.md](docs/RUNBOOK.md) for the full DigitalOcean install path.

At a high level:

1. Build Linux binaries.
2. Install `demo-api` and `demo-load` to `/usr/local/bin`.
3. Copy the systemd units from `deploy/systemd`.
4. Create `/etc/demo-observability/demo.env`.
5. Start both services.
6. Add `deploy/prometheus/demo-api-scrape.yml` to the existing Prometheus config.
7. Import `deploy/grafana/demo-app-dashboard.json` into Grafana.

## Configuration

`demo-api` reads:

- `DEMO_API_ADDR`: default `127.0.0.1:8080`
- `DEMO_ADMIN_TOKEN`: token required by `/chaos` when set
- `DEMO_TEMP_DIR`: directory for bounded temporary I/O work
- `DEMO_MAX_MEMORY_MB`: per-request memory allocation cap, default `48`

`demo-load` reads:

- `DEMO_TARGET_URL`: default `http://127.0.0.1:8080`
- `DEMO_ADMIN_TOKEN`: token used to update `/chaos`
- `DEMO_LOAD_PROFILE`: default `loop`
- `DEMO_LOAD_MAX_CONCURRENT`: default `12`

All settings can also be provided as CLI flags.

## Repository Layout

```text
cmd/demo-api/              API binary entrypoint
cmd/demo-load/             traffic generator entrypoint
internal/demoapp/          HTTP handlers, chaos state, metrics, bounded work
internal/loadgen/          traffic profiles and request generator
deploy/systemd/            systemd units and env example
deploy/prometheus/         scrape snippets and example config
deploy/grafana/            importable Grafana dashboard
docs/                      runbook and metrics reference
```

## Safety Notes

- Keep `DEMO_API_ADDR=127.0.0.1:8080` unless you intentionally put a private reverse proxy in front of it.
- Do not expose `/metrics` or `/chaos` publicly.
- Keep the systemd resource limits in place on the 512 MB droplet.
- The I/O endpoint writes temporary files and deletes them immediately.
- The memory endpoint allocates only up to `DEMO_MAX_MEMORY_MB` per request.

## Validation Checklist

Before calling the setup done:

```bash
go test ./...
go vet ./...
curl -s http://127.0.0.1:8080/metrics | grep '^demo_http_requests_total'
promtool check config /etc/prometheus/prometheus.yml
```

In Grafana, verify:

- the existing Node Exporter dashboard shows activity during the CPU/I/O phase.
- the new app dashboard has data for request rate, status codes, latency, workload phase, and chaos mode.
