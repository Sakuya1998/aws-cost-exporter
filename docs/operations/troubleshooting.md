# Troubleshooting

## Readiness and target isolation

`/ready` returns 503 with `missing` until every enabled Cost Explorer collector on every required target has succeeded. `stale` means its last success exceeded `cache.stale_after`. Optional targets, Organizations, Budgets, Commitments, Anomalies, Tags, and CUR do not gate readiness.

Inspect `aws_cost_exporter_collector_up{target="..."}` and `aws_cost_exporter_cache_age_seconds`. A failed target retains its old snapshot and does not block unrelated targets.

## AWS access, AssumeRole, and retries

For a Cost Explorer 403, confirm `ce:GetCostAndUsage` and `ce:GetCostForecast`. Organizations needs `organizations:ListAccounts` and `organizations:DescribeOrganization`; Budgets needs `budgets:ViewBudget`.

For credential failures, verify that the target references an existing source. A `profile` source must exist in `AWS_SHARED_CREDENTIALS_FILE`/`AWS_CONFIG_FILE`; headless SSO profiles need a valid cached login. A `static_env` source requires every configured environment variable to be present and non-empty.

Every target performs `sts:GetCallerIdentity` with its final credentials. A validation failure can mean that the selected Profile or static credentials belong to an account different from `target.account_id`. For AssumeRole failures, verify the exact role ARN, source trust principal, `sts:ExternalId`, target account ID, and `external_id_env`. Credential values, Caller ARN, and ExternalId are intentionally absent from logs.

For throttling, inspect `aws_cost_exporter_aws_api_requests_total`, `aws_cost_exporter_aws_api_retries_total`, and `aws_cost_exporter_pagination_pages_total` by target. SDK retries occur after the global and target attempt limiters. Scheduler backoff may retry the entire collector, but one scheduled run cannot exceed `collection.failure_backoff.max_attempts`. Reduce refresh frequency, filters, or `max_pages` before raising rate limits.

## Cost Explorer request cost

Cost Explorer logical calls, successful pages, SDK retries, and AWS-billed HTTP attempts are different quantities. A four-collector target normally performs 8 `GetCostAndUsage` operations per refresh before pagination, plus forecast. Billing data can be delayed or backfilled.

## Cardinality and unexpected values

Never aggregate different `currency` values. Forecast covers today through month end, so month-end estimates subtract today from MTD before adding forecast.

Never aggregate different `provider` or `cost_basis` values. `cost_explorer` is eventually consistent API data; `cur_athena` follows the CUR delivery schedule and fixed Athena query semantics.

Cost dimensions beyond `collection.cost_explorer.dimensions.series_limit` fold into `__other__`. Inspect `aws_cost_exporter_dimension_overflow_values_total`. `overflow_label` must not collide with a provider dimension.

Only AWS Budgets with `budget_type=COST` are exported. Non-currency usage and utilization budgets are rejected rather than mislabeled as currency metrics.

Organizations account metadata is limited to the explicit allowlist or observed linked accounts. Budgets is limited to configured names. Limits reject the refresh and retain old data rather than silently truncating it.

## Commitment, anomaly, and CUR failures

Commitment and anomaly collectors require the Cost Explorer permissions in `examples/iam/commitments-anomalies-readonly.json`. They expose bounded account-level summaries and never publish plan IDs, anomaly IDs, root causes, or raw AWS messages.

For CUR failures, verify `cur.region`, database, table, workgroup, result S3 location, unique tag column mappings, `collection.cur.max_currencies`, and `athena:StopQueryExecution` permission. The region must contain the configured Athena workgroup and Glue catalog. The S3 output location requires a valid lower-case bucket name and non-empty prefix. `query_timeout` must exceed the AWS request timeout, stay within one hour, and remain below the CUR refresh interval; `poll_interval` must be between 100ms and 1m. The exporter does not create tables or accept arbitrary SQL. If total and Tag results contain more distinct currencies than `max_currencies`, increase the bound only after increasing both CUR and Tag series budgets. Athena `FAILED`, `CANCELLED`, timed-out, malformed, duplicate-token, currency-limit, or other over-limit results are rejected atomically. Any abnormal exit before a terminal state triggers a best-effort stop. Inspect request metrics for `StartQueryExecution`, `GetQueryExecution`, `GetQueryResults`, and `StopQueryExecution`.

## Shutdown, replicas, and debug

Inspect `aws_cost_exporter_scheduler_shutdown_timeouts_total` if shutdown exceeds `server.shutdown_timeout`. Context cancellation should stop AWS waits, SDK retries, backoff, timers, and workers.

Keep one replica unless duplicate AWS requests and Prometheus targets are intentional. Debug endpoints are disabled by default and should be protected by a NetworkPolicy or authenticated proxy.
