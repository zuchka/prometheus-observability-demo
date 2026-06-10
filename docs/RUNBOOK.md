# DigitalOcean Runbook

This runbook assumes the droplet already runs Prometheus and node_exporter. It does not install or replace either service.

Run the commands on the droplet as a sudo-capable user, such as `deploy`. The examples assume the repository is cloned at `~/prometheus-observability-demo`.

## 1. Clone or Update the Repo

```bash
cd ~

if [ ! -d prometheus-observability-demo/.git ]; then
  git clone https://github.com/zuchka/prometheus-observability-demo.git
fi

cd ~/prometheus-observability-demo
git pull --ff-only
```

## 2. Build Binaries on the Droplet

For a typical Linux x86_64 DigitalOcean droplet:

```bash
cd ~/prometheus-observability-demo
mkdir -p bin/linux-amd64

GOOS=linux GOARCH=amd64 go build -o bin/linux-amd64/demo-api ./cmd/demo-api
GOOS=linux GOARCH=amd64 go build -o bin/linux-amd64/demo-load ./cmd/demo-load

file bin/linux-amd64/demo-api bin/linux-amd64/demo-load
```

If you are building natively on the droplet, this also works:

```bash
make build
```

The install commands below prefer the `bin/linux-amd64` outputs and fall back to the native `bin` outputs if you used `make build`.

## 3. Install the Demo Services

```bash
REPO_DIR="$HOME/prometheus-observability-demo"
DEMO_API_BIN="$REPO_DIR/bin/linux-amd64/demo-api"
DEMO_LOAD_BIN="$REPO_DIR/bin/linux-amd64/demo-load"

if [ ! -x "$DEMO_API_BIN" ]; then
  DEMO_API_BIN="$REPO_DIR/bin/demo-api"
fi

if [ ! -x "$DEMO_LOAD_BIN" ]; then
  DEMO_LOAD_BIN="$REPO_DIR/bin/demo-load"
fi

if [ ! -x "$DEMO_API_BIN" ] || [ ! -x "$DEMO_LOAD_BIN" ]; then
  echo "Could not find built demo binaries. Run the build commands in section 2 first." >&2
  exit 1
fi

if ! id -u demo-observability >/dev/null 2>&1; then
  sudo useradd --system --home /var/lib/demo-observability --shell /usr/sbin/nologin demo-observability
fi

sudo install -d -o root -g root -m 0755 /etc/demo-observability
sudo install -d -o demo-observability -g demo-observability -m 0755 /var/lib/demo-observability
sudo install -d -o demo-observability -g demo-observability -m 0755 /var/lib/demo-observability/tmp

sudo install -o root -g root -m 0755 "$DEMO_API_BIN" /usr/local/bin/demo-api
sudo install -o root -g root -m 0755 "$DEMO_LOAD_BIN" /usr/local/bin/demo-load

sudo install -o root -g root -m 0644 "$REPO_DIR/deploy/systemd/demo-api.service" /etc/systemd/system/demo-api.service
sudo install -o root -g root -m 0644 "$REPO_DIR/deploy/systemd/demo-load.service" /etc/systemd/system/demo-load.service
```

Create the environment file. This preserves an existing token if the file already exists:

```bash
REPO_DIR="$HOME/prometheus-observability-demo"

if [ ! -f /etc/demo-observability/demo.env ]; then
  TOKEN="$(openssl rand -hex 24)"
  sudo install -o root -g demo-observability -m 0640 "$REPO_DIR/deploy/systemd/demo.env.example" /etc/demo-observability/demo.env
  sudo sed -i "s/replace-with-a-long-random-token/${TOKEN}/" /etc/demo-observability/demo.env
else
  echo "/etc/demo-observability/demo.env already exists; leaving it unchanged"
fi
```

Start or restart the app and generator:

```bash
sudo systemctl daemon-reload
sudo systemctl enable demo-api.service demo-load.service
sudo systemctl restart demo-api.service demo-load.service
```

Check status:

```bash
sudo systemctl status demo-api.service --no-pager
sudo systemctl status demo-load.service --no-pager

curl -fsS http://127.0.0.1:8080/healthz && echo
curl -fsS http://127.0.0.1:8080/metrics | grep '^demo_chaos_mode'
```

## 4. Update Prometheus

This inserts the demo scrape job under the existing `scrape_configs` list in `/etc/prometheus/prometheus.yml`. If your Prometheus config lives somewhere else, change `PROM_CONFIG`.

```bash
REPO_DIR="$HOME/prometheus-observability-demo"
PROM_CONFIG="/etc/prometheus/prometheus.yml"

sudo cp "$PROM_CONFIG" "${PROM_CONFIG}.bak.$(date +%Y%m%d%H%M%S)"

if sudo grep -q 'job_name: demo_api' "$PROM_CONFIG"; then
  echo "demo_api scrape job already exists in $PROM_CONFIG"
else
  TMP_CONFIG="$(mktemp)"

  sudo awk -v snippet="$REPO_DIR/deploy/prometheus/demo-api-scrape.yml" '
    /^scrape_configs:[[:space:]]*$/ && inserted == 0 {
      print
      while ((getline line < snippet) > 0) {
        if (line !~ /^#/ && line !~ /^[[:space:]]*$/) {
          print "  " line
        }
      }
      close(snippet)
      inserted = 1
      next
    }
    { print }
  ' "$PROM_CONFIG" > "$TMP_CONFIG"

  if ! grep -q 'job_name: demo_api' "$TMP_CONFIG"; then
    echo "Could not insert demo_api scrape job. Confirm $PROM_CONFIG has a top-level scrape_configs: section." >&2
    rm -f "$TMP_CONFIG"
    exit 1
  fi

  sudo install -o root -g root -m 0644 "$TMP_CONFIG" "$PROM_CONFIG"
  rm -f "$TMP_CONFIG"
fi
```

Validate the final full config:

```bash
PROM_CONFIG="/etc/prometheus/prometheus.yml"
promtool check config "$PROM_CONFIG"
```

Reload Prometheus. If reload is not supported, the command falls back to the lifecycle endpoint, then to restart:

```bash
sudo systemctl reload prometheus \
  || curl -fsS -X POST http://127.0.0.1:9090/-/reload \
  || sudo systemctl restart prometheus
```

Confirm the target is up:

```bash
curl -fsS 'http://127.0.0.1:9090/api/v1/targets?state=active' | grep demo_api
```

## 5. Expose Prometheus with Caddy

Skip this section if Prometheus will only be used locally or through Grafana. Do not expose raw port `9090` publicly.

In Cloudflare DNS, create an `A` record:

```text
Type: A
Name: prometheus
Content: YOUR_DROPLET_PUBLIC_IPV4
Proxy status: DNS only
```

Keep the record set to DNS only until Caddy successfully gets a certificate. After HTTPS works, you may switch it to proxied and use Cloudflare SSL/TLS mode `Full (strict)`.

Install Caddy if needed:

```bash
if ! command -v caddy >/dev/null 2>&1; then
  sudo apt update
  sudo apt install -y debian-keyring debian-archive-keyring apt-transport-https curl
  curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | sudo gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
  curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' | sudo tee /etc/apt/sources.list.d/caddy-stable.list
  sudo chmod o+r /usr/share/keyrings/caddy-stable-archive-keyring.gpg
  sudo chmod o+r /etc/apt/sources.list.d/caddy-stable.list
  sudo apt update
  sudo apt install -y caddy
fi
```

Allow only SSH, HTTP, and HTTPS through UFW. Remove public `9090` access if it was previously allowed:

```bash
sudo ufw allow OpenSSH
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
# Run twice in case UFW has separate IPv4 and IPv6 rules.
sudo ufw delete allow 9090/tcp || true
sudo ufw delete allow 9090/tcp || true
sudo ufw --force enable
sudo ufw status verbose
```

Create a Caddy config with basic auth:

```bash
DOMAIN="prometheus.example.com"
HASH="$(caddy hash-password)"

sudo tee /etc/caddy/Caddyfile >/dev/null <<EOF
$DOMAIN {
    basic_auth {
        admin $HASH
    }

    reverse_proxy 127.0.0.1:9090
}
EOF

sudo caddy fmt --overwrite /etc/caddy/Caddyfile
sudo caddy validate --config /etc/caddy/Caddyfile
sudo systemctl reload caddy
```

If you intentionally want no password protection, replace the Caddyfile with:

```bash
DOMAIN="prometheus.example.com"

sudo tee /etc/caddy/Caddyfile >/dev/null <<EOF
$DOMAIN {
    reverse_proxy 127.0.0.1:9090
}
EOF

sudo caddy fmt --overwrite /etc/caddy/Caddyfile
sudo caddy validate --config /etc/caddy/Caddyfile
sudo systemctl reload caddy
```

Verify from the droplet:

```bash
curl -fsS http://127.0.0.1:9090/-/ready && echo
sudo ss -lntp | grep -E ':(80|443|9090)\b'
sudo journalctl -u caddy -n 120 --no-pager
```

Verify from your laptop:

```bash
DOMAIN="prometheus.example.com"
DROPLET_IP="YOUR_DROPLET_PUBLIC_IPV4"

curl -v --connect-timeout 10 --max-time 30 "http://$DOMAIN/"
curl -vk --connect-timeout 10 --max-time 30 --resolve "$DOMAIN:443:$DROPLET_IP" "https://$DOMAIN/"
```

## 6. Import Grafana Dashboard

In Grafana:

1. Open Dashboards.
2. Select New, then Import.
3. Upload `deploy/grafana/demo-app-dashboard.json`.
4. Select the Prometheus data source when prompted.
5. Set the time range to the last 30 minutes.

The existing Node Exporter dashboard should now show more visible CPU, load, memory, disk I/O, and process activity. The new app dashboard shows request rate, latency, errors, dependency latency, workload phase, and synthetic work counters.

## 7. Operate Safely

Resource limits are intentionally conservative:

- `demo-api.service`: `MemoryMax=160M`, `CPUQuota=70%`
- `demo-load.service`: `MemoryMax=96M`, `CPUQuota=50%`
- `/api/memory`: capped by `DEMO_MAX_MEMORY_MB`, default `48`
- `/api/io`: capped to 4 MiB per request
- `/api/cpu`: capped to 350 ms per request

The demo API listens on `127.0.0.1:8080` by default. Do not expose this service directly to the internet. Public users should see Grafana, Prometheus behind Caddy, or another dashboard surface, not `/metrics` or `/chaos`.

## 8. Troubleshooting

No app metrics in Prometheus:

```bash
curl -fsS http://127.0.0.1:8080/metrics | head
curl -fsS 'http://127.0.0.1:9090/api/v1/targets?state=active' | grep demo_api
sudo journalctl -u prometheus -n 100 --no-pager
```

Traffic generator cannot set chaos mode:

```bash
sudo journalctl -u demo-load -n 100 --no-pager
sudo grep DEMO_ADMIN_TOKEN /etc/demo-observability/demo.env
sudo systemctl restart demo-load
```

Cloudflare returns 522:

```bash
DOMAIN="prometheus.example.com"
DROPLET_IP="YOUR_DROPLET_PUBLIC_IPV4"

dig +short "$DOMAIN"
curl -vk --connect-timeout 10 --max-time 30 --resolve "$DOMAIN:443:$DROPLET_IP" "https://$DOMAIN/"
curl -v --connect-timeout 10 --max-time 30 --resolve "$DOMAIN:80:$DROPLET_IP" "http://$DOMAIN/"
sudo ss -lntp | grep -E ':(80|443|9090)\b'
sudo ufw status verbose
sudo journalctl -u caddy -n 120 --no-pager
```

Most 522s during this setup are caused by one of these:

- Cloudflare DNS is proxied before Caddy gets a certificate.
- UFW does not allow `80/tcp` and `443/tcp`.
- The DNS record points to the wrong droplet IP.

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
