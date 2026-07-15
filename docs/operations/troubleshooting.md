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
throttling or 5xx responses, inspect `aws_cost_exporter_aws_api_requests_total`
(including `status="throttle"`), `aws_cost_exporter_aws_api_retries_total`,
and `aws_cost_exporter_pagination_pages_total`. The SDK retries throttled
requests before the scheduler applies failure backoff and may rerun the entire
collector refresh. `pagination_pages_total` counts page reads per query;
`aws_api_requests_total` counts each HTTP attempt including SDK retries. When
both spike, reduce refresh frequency, tighten filters, or lower `max_pages`
before raising rate limits.

## Unexpected cost data

Cost Explorer can deliver delayed backfill, so `AWSDailyCostSpike` may fire
after AWS revises data; tune its 2h duration around the business calendar.
Never aggregate different `currency` values. The daily gauge is UTC scrape
history, not historical billing rows. Forecast covers today through month end;
month-end estimates must subtract today's amount from MTD before adding it.

Dimension values beyond the configured limit are folded into `__other__`
without losing totals. `series_limit` caps exported Prometheus series only;
Cost Explorer may still return more groups. Inspect
`aws_cost_exporter_dimension_overflow_values_total` and
`aws_cost_exporter_pagination_pages_total` before raising limits. Pagination
beyond `cost_explorer.max_pages` fails the collector refresh.

## Deployment

Keep one replica unless duplicate AWS calls and Prometheus targets are intended.
The debug endpoints are disabled by default; when enabled, protect them with a
NetworkPolicy or authenticated proxy. Logs go to stdout/stderr; see
[logging.md](logging.md) for rotation and retention.
