[English](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Troubleshooting-and-Logging) | [简体中文](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Troubleshooting-and-Logging-zh-CN)

# Troubleshooting and Logging

## `/ready` is 503

`missing` means an enabled Cost Explorer collector on a required target has never succeeded. `stale` means the last success exceeded `cache.stale_after`. Inspect `aws_cost_exporter_collector_up`, `aws_cost_exporter_cache_age_seconds`, and last-success timestamps by `target` and `collector`.

Optional collectors do not gate readiness. A failed collector retains old data, so a 200 `/metrics` response does not prove freshness.

## AWS access and identity

For a Cost Explorer 403, verify `ce:GetCostAndUsage` and `ce:GetCostForecast`. Then verify the final credential account matches `target.account_id`; every target performs STS `GetCallerIdentity` before billing calls.

For Profile failures, check shared AWS files and headless SSO login. For AssumeRole, check the exact role ARN, source principal, `sts:ExternalId`, and `external_id_env`. Sensitive values and caller ARNs are deliberately absent from logs.

## Throttling and request cost

For throttling, inspect `aws_cost_exporter_aws_api_requests_total`, `aws_cost_exporter_aws_api_retries_total`, and `aws_cost_exporter_pagination_pages_total`. SDK retries are limited after the global and target token waits. Reduce filters, pages, enabled bases, and refresh frequency before raising RPS.

Billing data can be delayed or backfill earlier values. Forecast covers today through month end; subtract today's daily amount from MTD before adding forecast.

## Cardinality

Values beyond limits fold into `__other__`; inspect `aws_cost_exporter_dimension_overflow_values_total`. `overflow_label` must not collide with provider or real dimension values. Never merge different `currency`, provider, or basis values. Increase `max_currencies` only after increasing compatible CUR and Tag series budgets.

## CUR and Athena

Verify CUR region, workgroup, Glue table, input prefix, result prefix, `query_timeout`, `poll_interval`, row/page limits, and `athena:StopQueryExecution`. Inspect operations `StartQueryExecution`, `GetQueryExecution`, `GetQueryResults`, and `StopQueryExecution`. Failed or malformed results retain old data.

## Shutdown, replica, and debug

Inspect `aws_cost_exporter_scheduler_shutdown_timeouts_total` if shutdown exceeds the configured timeout. Keep one replica unless duplicate calls are accepted. Debug endpoints are disabled by default and must be protected by a NetworkPolicy or authenticated proxy when enabled.

## Logging and rotation

The exporter writes structured JSON or text logs to stdout/stderr. It does not create, reopen, rotate, compress, or delete files.

- Docker: configure the `local` or `json-file` driver with bounded size and count.
- Kubernetes: configure kubelet rotation and ship logs with a cluster agent.
- systemd: use journald and central retention limits.
- Direct file redirection: use external logrotate only when unavoidable.

Restrict log access because cost metadata can reveal account structure. Never log credentials, authorization headers, ExternalId, account email, or raw AWS responses.
