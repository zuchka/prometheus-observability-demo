# Metrics Reference

All application metrics use the `demo_` prefix. Prometheus should scrape `http://127.0.0.1:8080/metrics`.

## HTTP

- `demo_http_requests_total{method,route,status}`: request counter with stable route labels.
- `demo_http_request_duration_seconds_bucket{method,route,le}`: request duration histogram.
- `demo_http_requests_in_flight`: current request concurrency.

## Workload State

- `demo_chaos_mode`: current synthetic behavior mode.
  - `0`: normal
  - `1`: higher error rate
  - `2`: higher latency
  - `3`: mixed degraded mode
- `demo_workload_phase{phase}`: active traffic-generator phase has value `1`; inactive phases have value `0`.
- `demo_workload_events_total{phase}`: phase change counter.

## Synthetic Dependencies

- `demo_dependency_duration_seconds_bucket{dependency,le}`: latency histograms for fake dependencies such as `cache`, `database`, `search`, and `payments`.

## Host-Visible Work

These are app-level counters that correspond to activity visible in the existing Node Exporter dashboard.

- `demo_cpu_work_seconds_total`: seconds spent doing bounded CPU work.
- `demo_cpu_work_iterations_total`: loop iterations completed by bounded CPU work.
- `demo_io_work_bytes_total`: bytes written and synced to temporary files.
- `demo_memory_work_bytes_total`: bytes allocated by bounded memory work.

## Useful PromQL

Request rate:

```promql
sum by (route) (rate(demo_http_requests_total[1m]))
```

Error ratio:

```promql
sum(rate(demo_http_requests_total{status=~"5.."}[5m]))
/
clamp_min(sum(rate(demo_http_requests_total[5m])), 0.001)
```

P95 request latency:

```promql
histogram_quantile(
  0.95,
  sum by (le, route) (rate(demo_http_request_duration_seconds_bucket[5m]))
)
```

P95 dependency latency:

```promql
histogram_quantile(
  0.95,
  sum by (le, dependency) (rate(demo_dependency_duration_seconds_bucket[5m]))
)
```

