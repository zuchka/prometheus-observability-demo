# DigitalOcean Runbook

This runbook assumes the droplet already runs Prometheus and node_exporter. It does not install or replace either service.

## 1. Build Binaries

On a machine with Go installed:

```bash
make build
```

For a typical Linux x86_64 DigitalOcean droplet, cross-compile from macOS with:

```bash
GOOS=linux GOARCH=amd64 go build -o bin/linux-amd64/demo-api ./cmd/demo-api
GOOS=linux GOARCH=amd64 go build -o bin/linux-amd64/demo-load ./cmd/demo-load
```

Copy the binaries and deployment files to the droplet:

```bash
scp bin/linux-amd64/demo-api bin/linux-amd64/demo-load root@YOUR_DROPLET:/tmp/
scp deploy/systemd/demo-api.service deploy/systemd/demo-load.service deploy/systemd/demo.env.example root@YOUR_DROPLET:/tmp/
scp deploy/prometheus/demo-api-scrape.yml deploy/grafana/demo-app-dashboard.json root@YOUR_DROPLET:/tmp/
```

## 2. Install Services

Run these commands on the droplet:

```bash
sudo useradd --system --home /var/lib/demo-observability --shell /usr/sbin/nologin demo-observability || true
sudo mkdir -p /etc/demo-observability /var/lib/demo-observability/tmp
sudo chown -R demo-observability:demo-observability /var/lib/demo-observability

sudo install -o root -g root -m 0755 /tmp/demo-api /usr/local/bin/demo-api
sudo install -o root -g root -m 0755 /tmp/demo-load /usr/local/bin/demo-load
sudo install -o root -g root -m 0644 /tmp/demo-api.service /etc/systemd/system/demo-api.service
sudo install -o root -g root -m 0644 /tmp/demo-load.service /etc/systemd/system/demo-load.service
```

Create the environment file:

```bash
TOKEN="$(openssl rand -hex 24)"
sudo install -o root -g demo-observability -m 0640 /tmp/demo.env.example /etc/demo-observability/demo.env
sudo sed -i "s/replace-with-a-long-random-token/${TOKEN}/" /etc/demo-observability/demo.env
```

Start the app and generator:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now demo-api.service
sudo systemctl enable --now demo-load.service
```

Check status:

```bash
sudo systemctl status demo-api.service --no-pager
sudo systemctl status demo-load.service --no-pager
curl -s http://127.0.0.1:8080/healthz
curl -s http://127.0.0.1:8080/metrics | grep '^demo_chaos_mode'
```

## 3. Update Prometheus

Add the contents of `deploy/prometheus/demo-api-scrape.yml` under the existing `scrape_configs` list in your Prometheus config.

Validate the final full config:

```bash
promtool check config /etc/prometheus/prometheus.yml
```

Reload Prometheus using the method your install supports:

```bash
sudo systemctl reload prometheus
```

If reload is not supported but the web lifecycle endpoint is enabled:

```bash
curl -X POST http://127.0.0.1:9090/-/reload
```

If neither works:

```bash
sudo systemctl restart prometheus
```

Confirm the target is up:

```bash
curl -s 'http://127.0.0.1:9090/api/v1/targets' | grep demo_api
```

## 4. Import Grafana Dashboard

In Grafana:

1. Open Dashboards.
2. Select New, then Import.
3. Upload `deploy/grafana/demo-app-dashboard.json`.
4. Select the Prometheus data source when prompted.
5. Set the time range to the last 30 minutes.

The existing Node Exporter dashboard should now show more visible CPU, load, memory, disk I/O, and process activity. The new app dashboard shows request rate, latency, errors, dependency latency, workload phase, and synthetic work counters.

## 5. Operate Safely

Resource limits are intentionally conservative:

- `demo-api.service`: `MemoryMax=160M`, `CPUQuota=70%`
- `demo-load.service`: `MemoryMax=96M`, `CPUQuota=50%`
- `/api/memory`: capped by `DEMO_MAX_MEMORY_MB`, default `48`
- `/api/io`: capped to 4 MiB per request
- `/api/cpu`: capped to 350 ms per request

The app listens on `127.0.0.1:8080` by default. Do not expose this service directly to the internet. Public users should see Grafana or another dashboard surface, not `/metrics` or `/chaos`.

## 6. Troubleshooting

No app metrics in Prometheus:

```bash
curl -s http://127.0.0.1:8080/metrics | head
curl -s 'http://127.0.0.1:9090/api/v1/targets' | grep demo_api
sudo journalctl -u prometheus -n 100 --no-pager
```

Traffic generator cannot set chaos mode:

```bash
sudo journalctl -u demo-load -n 100 --no-pager
sudo grep DEMO_ADMIN_TOKEN /etc/demo-observability/demo.env
sudo systemctl restart demo-load
```

The node dashboard is still flat:

- Confirm `demo-load.service` is running.
- Confirm the Node Exporter dashboard is looking at the correct instance.
- Wait through a full loop. The CPU/I/O phase occurs after baseline, burst, error, and latency phases.
- Temporarily run a stronger local pulse:

```bash
sudo -u demo-observability DEMO_ADMIN_TOKEN="$(sudo awk -F= '/DEMO_ADMIN_TOKEN/ {print $2}' /etc/demo-observability/demo.env)" /usr/local/bin/demo-load -profile=cpu-io-pulse -once -step-duration=60s
```

Stop the demo:

```bash
sudo systemctl disable --now demo-load.service demo-api.service
```
