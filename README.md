# AWS Cost Exporter

[github.com/sakuya1998/aws-cost-exporter](https://github.com/sakuya1998/aws-cost-exporter) exports cached AWS billing data for Prometheus. It is an exporter, not a financial reconciliation system.

v0.2 operates across explicitly configured AWS account targets. Each target selects a named default-chain, shared-profile, or environment-backed credential source and may optionally AssumeRole before enabling Cost Explorer, Organizations metadata, and allowlisted AWS Budgets.

Prometheus scrapes only an immutable in-memory snapshot and does not call AWS during a Prometheus scrape.

## Install

Build locally:

```bash
make build
./aws-cost-exporter --config configs/aws-cost-exporter.example.yaml
```

Container:

```bash
docker pull ghcr.io/sakuya1998/aws-cost-exporter:latest
docker compose up --build
```

Helm OCI chart:

```bash
helm install aws-cost-exporter \
  oci://ghcr.io/sakuya1998/charts/aws-cost-exporter \
  --set config.data.targets[0].account_id=444455556666
```

The chart intentionally defaults to `replicaCount: 1`. Multiple replicas duplicate AWS requests and expose duplicate scrape targets because v0.2 has no leader election or shared cache. If the exporter listen port is changed, set `service.targetPort` to the same port; Helm rejects mismatched generated configurations.

## Configuration

The only accepted top-level keys are:

```text
server
log
aws
targets
collection
cache
telemetry
```

There is no `config_version`, v0.1 compatibility parser, automatic migration, or deprecated alias. Unknown fields are rejected.

Start with [configs/aws-cost-exporter.example.yaml](configs/aws-cost-exporter.example.yaml). At least one of 1–20 targets must be `required` and enable Cost Explorer.

```yaml
aws:
  region: us-east-1
  request_timeout: 30s
  credentials:
    sources:
      runtime:
        type: default_chain
      account-a:
        type: profile
        profile: account-a
      legacy:
        type: static_env
        access_key_id_env: LEGACY_AWS_ACCESS_KEY_ID
        secret_access_key_env: LEGACY_AWS_SECRET_ACCESS_KEY
  endpoints:
    sts: ""
    cost_explorer: ""
    organizations: ""
    budgets: ""
  retry:
    max_attempts: 5
    base_delay: 1s
    max_backoff: 30s
  rate_limit:
    global_requests_per_second: 1
    global_burst: 2
    target_requests_per_second: 0.5
    target_burst: 1

targets:
  - name: payer-prod
    account_id: "444455556666"
    required: true
    credentials:
      source: runtime
      assume_role:
        role_arn: arn:aws:iam::444455556666:role/aws-cost-exporter-reader
        external_id_env: AWS_COST_EXPORTER_PAYER_PROD_EXTERNAL_ID
        session_name: aws-cost-exporter-payer-prod
    cost_explorer:
      enabled: true
      filters:
        linked_account_ids: []
        services: []
        regions: []
    organizations:
      enabled: true
      account_ids: []
    budgets:
      enabled: true
      names: [Monthly-Production]
```

Credential source types are `default_chain`, `profile`, and `static_env`. Every target explicitly references one source. A source may back many AssumeRole targets but at most one direct target. Account IDs and Role ARNs are unique. Profile sources use standard AWS shared configuration behavior, including `source_profile`, SSO, `credential_process`, and profile role chains.

`static_env` and `external_id_env` contain environment-variable names, never secret values. Referenced variables must exist and be non-empty during `--check-config` and production startup. AK, SK, session tokens, and ExternalId values never appear in configuration, ConfigMaps, logs, metrics, or debug endpoints. AssumeRole ARNs must identify one exact role without wildcards and match the target `account_id`.

Before a collector accesses billing APIs, its final target credentials are verified with STS `GetCallerIdentity`. A mismatched Profile or credential source cannot publish costs under the wrong target. Verification is target-scoped, single-flight, cached after success, and uses the normal global and target request limiters.

Organizations `account_ids` is an optional allowlist. When non-empty, only those accounts are exported. When empty, metadata is exported only for linked accounts already observed by the target account cost collector. Organizations never creates targets automatically and never exposes account email.

Budgets requires a non-empty exact-name allowlist and exports only `COST` budgets. Usage, RI, and Savings Plans utilization/coverage budgets use non-currency units and are rejected. Missing actual or forecasted values omit the corresponding series instead of exporting zero.

Important bounds:

- `targets` and credential sources: 1–20.
- `aws.retry.max_attempts`: 1–10.
- `collection.failure_backoff.max_attempts`: 1–10 logical collector attempts per scheduled run; the default is 3.
- global and target burst: 1–5; RPS must be finite, positive, and no more than 10.
- all `max_pages`: 1–200.
- Cost Explorer and Organizations `series_limit`: at most 2000; Budgets: at most 500.
- `collection.jitter_ratio`: finite and 0–0.5.
- `collection.cost_explorer.cost_metric`: only `UnblendedCost`.
- `collection.cost_explorer.dimensions.overflow_label` must already be trimmed and must not collide with provider values.
- `server.shutdown_timeout` and all other timeouts must be positive.

Validate the exact production configuration and every referenced credential or ExternalId environment variable:

```bash
./aws-cost-exporter --config config.yaml --check-config
```

CLI flags:

```text
--config
--listen-address
--log-level
--check-config
--version
```

Environment overrides use double underscores, for example `AWS_COST_EXPORTER_SERVER__LISTEN_ADDRESS=:9090`.

For Helm, use `config.secretEnvRefs` to inject `static_env` credentials and ExternalId values from existing Secrets. Use `awsSharedConfig.existingSecret` to mount AWS `credentials` and `config` files; the chart sets `AWS_SHARED_CREDENTIALS_FILE` and `AWS_CONFIG_FILE`. Secret values never enter the generated ConfigMap. For Docker Compose, the read-only `${HOME}/.aws` mount supplies named Profile sources.

## AWS permissions

Cost Explorer targets require:

```text
ce:GetCostAndUsage
ce:GetCostForecast
```

The exporter calls STS `GetCallerIdentity` for every final target identity; AWS does not normally require an explicit allow for that operation. Organizations targets additionally require `organizations:ListAccounts` and `organizations:DescribeOrganization`. Budgets targets require `budgets:ViewBudget`. A source principal requires `sts:AssumeRole` only for each exact configured role ARN.

See [examples/iam](examples/iam). Static access key values are never configuration fields; only environment-variable names may be configured.

## AWS request cost and limits

Cost Explorer API requests are billed by AWS, currently USD 0.01 per billable request. Verify current AWS pricing before deployment.

With total, service, region, and account collectors enabled, one unpaginated refresh uses 8 `GetCostAndUsage` logical operations: daily and month-to-date for four collectors. Forecast adds one `GetCostForecast`. For `T` targets and `P` average pages:

```text
approximate Cost Explorer requests per refresh = T × (8 × P + forecast operations)
```

SDK retries can produce additional billable HTTP attempts. `aws_cost_exporter_aws_api_requests_total` counts logical SDK operations, `aws_cost_exporter_pagination_pages_total` counts successfully read pages, and `aws_cost_exporter_aws_api_retries_total` counts authorized retry attempts. They are related but not interchangeable with the AWS invoice.

Both initial attempts and SDK retries acquire the global limiter and then the target limiter. A scheduled collector run is additionally bounded by `collection.failure_backoff.max_attempts`; after that budget is exhausted it waits for the next normal interval. Collectors for one target run serially so a slow account cannot occupy every global collection slot. Tighten target filters, increase refresh intervals, or reduce `max_pages` before raising rate limits. `series_limit` bounds Prometheus cardinality but does not reduce Cost Explorer pages.

Use `AWSCostExplorerPaginationSpike` and `AWSCostExplorerThrottleSustained` from [rules/prometheus/aws-cost-exporter.rules.yaml](rules/prometheus/aws-cost-exporter.rules.yaml).

## HTTP endpoints

```bash
curl http://localhost:8080/metrics
curl http://localhost:8080/healthz
curl http://localhost:8080/ready
curl http://localhost:8080/version
```

`/healthz` represents process liveness. `/ready` requires every enabled Cost Explorer collector, including forecast, on every required target to have a non-stale successful snapshot. Optional targets, Organizations, and Budgets do not gate readiness. Old snapshots remain available after refresh failures.

Debug endpoints are disabled by default and never expose credentials or configuration secrets.

## Metrics

Every business and target-scoped operational metric has a mandatory `target` label. Never sum different `currency` values.

Cost metrics:

```text
aws_cost_daily_amount
aws_cost_month_to_date_amount
aws_cost_service_daily_amount
aws_cost_service_month_to_date_amount
aws_cost_region_daily_amount
aws_cost_region_month_to_date_amount
aws_cost_account_daily_amount
aws_cost_account_month_to_date_amount
aws_cost_month_forecast_mean_amount
aws_cost_month_forecast_lower_bound_amount
aws_cost_month_forecast_upper_bound_amount
aws_cost_account_info
```

Budgets metrics:

```text
aws_budget_limit_amount
aws_budget_actual_amount
aws_budget_forecasted_amount
```

Budget labels are `target`, `budget_name`, `currency`, `budget_type`, and `time_unit`.

Exporter metrics:

```text
aws_cost_exporter_collector_up
aws_cost_exporter_last_success_timestamp_seconds
aws_cost_exporter_last_attempt_timestamp_seconds
aws_cost_exporter_cache_age_seconds
aws_cost_exporter_snapshot_series
aws_cost_exporter_refresh_total
aws_cost_exporter_refresh_duration_seconds
aws_cost_exporter_aws_api_requests_total
aws_cost_exporter_aws_api_retries_total
aws_cost_exporter_aws_api_request_duration_seconds
aws_cost_exporter_scheduler_skipped_runs_total
aws_cost_exporter_dimension_overflow_values_total
aws_cost_exporter_pagination_pages_total
aws_cost_exporter_cache_publish_errors_total
aws_cost_exporter_build_info
aws_cost_exporter_scheduler_shutdown_timeouts_total
```

`aws_cost_exporter_build_info` and `aws_cost_exporter_scheduler_shutdown_timeouts_total` are process-global. All other exporter metrics above are target-scoped.

Dimension values beyond `collection.cost_explorer.dimensions.series_limit` are aggregated into `overflow_label`, normally `__other__`, while preserving monetary totals.

## Dashboards and operations

- [dashboards/grafana/aws-cost-exporter.json](dashboards/grafana/aws-cost-exporter.json)
- [rules/prometheus/aws-cost-exporter.rules.yaml](rules/prometheus/aws-cost-exporter.rules.yaml)
- [docs/operations/troubleshooting.md](docs/operations/troubleshooting.md)
- [ARCHITECTURE.md](ARCHITECTURE.md)

The dashboard derives target selection from exporter collector status, so it also works when the total-cost collector is disabled. It keeps target and currency in all monetary aggregations.

## Development

```bash
go test ./...
go test -race ./...
go vet ./...
golangci-lint run
```

The repository is licensed under the [Apache License 2.0](LICENSE).
