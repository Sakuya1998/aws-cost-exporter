# Troubleshooting

## Readiness and stale data

`/ready` returns 503 with `missing` before every required collector has a first
snapshot or after a required collector fails without usable data. A `stale`
reason means the last snapshot exceeded `cache.stale_after` (24h by default).
Old values remain on `/metrics`. `aws_cost_exporter_collector_up` describes the
latest attempt, while `aws_cost_exporter_cache_age_seconds` measures data age.

## AWS access and retries

For 403 errors, confirm Cost Explorer is enabled and the runtime identity has
the two actions in `examples/iam/mvp-readonly.json`. Region must be
`us-east-1`. Authorization failures are not retried aggressively. For
throttling or 5xx responses, inspect `aws_cost_exporter_aws_api_requests_total`,
retry counters, and request-duration histograms; the SDK retries before the
scheduler applies failure backoff.

## Unexpected cost data

Cost Explorer can deliver delayed backfill, so `AWSDailyCostSpike` may fire
after AWS revises data; tune its 2h duration around the business calendar.
Never aggregate different `currency` values. The daily gauge is UTC scrape
history, not historical billing rows. Forecast covers today through month end;
month-end estimates must subtract today's amount from MTD before adding it.

Dimension values beyond the configured limit are folded into `__other__`
without losing totals. Inspect
`aws_cost_exporter_dimension_overflow_values_total` before raising the limit.

## Deployment

Keep one replica unless duplicate AWS calls and Prometheus targets are intended.
The debug endpoints are disabled by default; when enabled, protect them with a
NetworkPolicy or authenticated proxy. Logs go to stdout/stderr; see
[logging.md](logging.md) for rotation and retention.
