# AWS Cost Exporter

[github.com/sakuya1998/aws-cost-exporter](https://github.com/sakuya1998/aws-cost-exporter) exports cached AWS billing data for Prometheus. It is an exporter, not a financial reconciliation system.

v0.3 operates across explicitly configured AWS account targets. In addition to Cost Explorer, Organizations, and Budgets, a target can expose bounded Savings Plans/Reserved Instance summaries, Cost Anomaly Detection summaries, allowlisted tag costs, and fixed-schema CUR 2.0 data through Athena.

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

The chart intentionally defaults to `replicaCount: 1`. Multiple replicas duplicate AWS and Athena requests and expose duplicate scrape targets because v0.3 has no leader election or shared cache. If the exporter listen port is changed, set `service.targetPort` to the same port; Helm rejects mismatched generated configurations.

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

There is no `config_version`, v0.2 compatibility parser, automatic migration, or deprecated alias. Unknown fields are rejected.

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
    commitments:
      enabled: false
    anomalies:
      enabled: false
    tags:
      enabled: false
      keys: []
    cur:
      enabled: false
      region: us-east-1
      database: ""
      table: ""
      workgroup: ""
      output_location: ""
      query_timeout: 10m
      poll_interval: 2s
      tag_columns: []
```

Credential source types are `default_chain`, `profile`, and `static_env`. Every target explicitly references one source. A source may back many AssumeRole targets but at most one direct target. Account IDs and Role ARNs are unique. Profile sources use standard AWS shared configuration behavior, including `source_profile`, SSO, `credential_process`, and profile role chains.

`static_env` and `external_id_env` contain environment-variable names, never secret values. Referenced variables must exist and be non-empty during `--check-config` and production startup. AK, SK, session tokens, and ExternalId values never appear in configuration, ConfigMaps, logs, metrics, or debug endpoints. AssumeRole ARNs must identify one exact role without wildcards and match the target `account_id`.

Before a collector accesses billing APIs, its final target credentials are verified with STS `GetCallerIdentity`. A mismatched Profile or credential source cannot publish costs under the wrong target. Verification is target-scoped, single-flight, cached after success, and uses the normal global and target request limiters.

Organizations `account_ids` is an optional allowlist. When non-empty, only those accounts are exported. When empty, metadata is exported only for linked accounts already observed by the target account cost collector. Organizations never creates targets automatically and never exposes account email.

Budgets requires a non-empty exact-name allowlist and exports only `COST` budgets. Usage, RI, and Savings Plans utilization/coverage budgets use non-currency units and are rejected. Missing actual or forecasted values omit the corresponding series instead of exporting zero.

`collection.cost_explorer.cost_bases` is a non-empty subset of `unblended`, `amortized`, and `net`. Every cost series includes `provider` and `cost_basis`; do not sum across either label. Commitment and anomaly collectors expose account-level summaries only and never use plan IDs, anomaly IDs, service names, root causes, or raw AWS messages as labels.

CUR requires a pre-created CUR 2.0 table and Athena workgroup. The Athena workgroup ARN region must match `targets[].cur.region`, and that region must also contain the Glue catalog; this is independent from the Cost Explorer `aws.region`, which remains `us-east-1`. The exporter generates fixed read-only queries and polls Athena asynchronously; arbitrary SQL is not accepted. Each totals query and Tag query references the CUR table once and expands its bounded window, basis, and tag dimensions inside Athena. Configure `cur.tag_columns` to map each allowlisted public tag key to one validated CUR column. `collection.cur.max_currencies` bounds the distinct currency union returned by total and Tag queries; exceeding it rejects the complete refresh and retains the previous snapshot. Failed, canceled, timed-out, over-limit, or malformed queries retain the previous snapshot.

CUR amortized cost uses AWS CUR 2.0 line-item semantics: covered usage uses Savings Plans or RI effective cost, Savings Plans recurring fees contribute unused commitment, RI fee rows contribute unused amortized upfront and recurring fees, and Savings Plans negation/upfront plus reservation upfront fee rows are excluded to prevent double counting. Other rows use unblended cost. Net cost uses `line_item_net_unblended_cost` with unblended fallback for rows where AWS leaves the net column empty. These bases remain separate series and must not be added together.

Important bounds:

- `server.max_in_flight`: 1..1000; `collection.max_concurrency`: 1..100.
- The Cost Explorer Tag worst case is `sum(keys[].max_values) * 2 windows * number of cost bases` and must not exceed `collection.tags.series_limit`.
- The CUR Tag worst case multiplies that budget by `collection.cur.max_currencies`; it must still fit within `collection.tags.series_limit`.
- For a CUR target, `max_currencies * (2 total-cost windows * cost bases + per-currency Tag budget)` must fit within `collection.cur.series_limit`.
- `cur.tag_columns` keys and physical CUR column names must both be unique.
- `targets[].cur.query_timeout` must exceed `aws.request_timeout`, must not exceed 1 hour, and must remain below `collection.cur.refresh_interval`; `poll_interval` is limited to 100ms..1m.
- `targets[].cur.output_location` must use a valid lower-case S3 bucket name and a non-empty prefix.

- `targets` and credential sources: 1–20.
- `aws.retry.max_attempts`: 1–10.
- `collection.failure_backoff.max_attempts`: 1–10 logical collector attempts per scheduled run; the default is 3.
- global and target burst: 1–5; RPS must be finite, positive, and no more than 10.
- all `max_pages`: 1–200.
- Cost Explorer and Organizations `series_limit`: at most 2000; Budgets: at most 500.
- Tag keys: at most 20 per target; each `max_values` is 1..500 and overflow is aggregated.
- CUR: at most 200 pages, 100,000 rows, 1..10 currencies, and 20,000 series per collector refresh.
- `collection.jitter_ratio`: finite and 0–0.5.
- `collection.cost_explorer.cost_bases`: unique subset of `unblended`, `amortized`, and `net`.
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

The exporter calls STS `GetCallerIdentity` for every final target identity; AWS does not normally require an explicit allow for that operation. Organizations targets additionally require `organizations:ListAccounts` and `organizations:DescribeOrganization`. Budgets targets require `budgets:ViewBudget`. Commitment and anomaly targets require the corresponding Cost Explorer read APIs. CUR targets require `athena:StartQueryExecution`, `athena:GetQueryExecution`, `athena:GetQueryResults`, `athena:StopQueryExecution`, scoped Glue catalog reads, CUR prefix reads, and access to the configured Athena result S3 prefix. Replace every account, region, database, table, bucket, and prefix placeholder in `examples/iam/cur-athena-readonly.json`. A source principal requires `sts:AssumeRole` only for each exact configured role ARN.

See [examples/iam](examples/iam). Static access key values are never configuration fields; only environment-variable names may be configured.

## AWS request cost and limits

Cost Explorer API requests are billed by AWS, currently USD 0.01 per billable request. Verify current AWS pricing before deployment.

With total, service, region, and account collectors enabled, one unpaginated refresh uses 8 `GetCostAndUsage` logical operations per configured cost basis. Forecast uses the first configured basis and adds one `GetCostForecast`. Each allowlisted Cost Explorer tag key adds two operations per basis. For `T` targets, `B` bases, `K` tag keys, and `P` average pages:

```text
approximate Cost Explorer requests per refresh = T * (((8 + 2*K) * B * P) + forecast operations + commitment/anomaly operations)
```

SDK retries can produce additional billable HTTP attempts. `aws_cost_exporter_aws_api_requests_total` counts logical SDK operations, `aws_cost_exporter_pagination_pages_total` counts successfully read pages, and `aws_cost_exporter_aws_api_retries_total` counts authorized retry attempts. They are related but not interchangeable with the AWS invoice.

Athena is billed by data scanned. The exporter submits fixed aggregate queries only on the configured CUR refresh interval, never during scrape. Totals and Tag collection are separate queries, but each query contains only one CUR table reference. `query_timeout` covers submission, polling, and result pagination; any abnormal exit before Athena reports a terminal state triggers a best-effort `StopQueryExecution`. Partition the CUR table by billing period, use a dedicated workgroup with scan limits, and monitor Athena costs before reducing the default 24-hour interval.

Both initial attempts and SDK retries acquire the global limiter and then the target limiter. A scheduled collector run is additionally bounded by `collection.failure_backoff.max_attempts`; after that budget is exhausted it waits for the next normal interval. Collectors for one target run serially so a slow account cannot occupy every global collection slot. Tighten target filters, increase refresh intervals, or reduce `max_pages` before raising rate limits. `series_limit` bounds Prometheus cardinality but does not reduce Cost Explorer pages.

Use `AWSCostExplorerPaginationSpike` and `AWSCostExplorerThrottleSustained` from [rules/prometheus/aws-cost-exporter.rules.yaml](rules/prometheus/aws-cost-exporter.rules.yaml).

## HTTP endpoints

```bash
curl http://localhost:8080/metrics
curl http://localhost:8080/healthz
curl http://localhost:8080/ready
curl http://localhost:8080/version
```

`/healthz` represents process liveness. `/ready` requires every enabled Cost Explorer collector, including forecast, on every required target to have a non-stale successful snapshot. Optional targets, Organizations, Budgets, Commitments, Anomalies, Tags, and CUR do not gate readiness. Old snapshots remain available after refresh failures.

Debug endpoints are disabled by default and never expose credentials or configuration secrets.

## Metrics

Every business and target-scoped operational metric has a mandatory `target` label. Cost metrics also use fixed `provider` and `cost_basis` labels. Never sum different `currency`, `provider`, or `cost_basis` values.

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
aws_cost_tag_daily_amount
aws_cost_tag_month_to_date_amount
```

Commitment and anomaly metrics:

```text
aws_commitment_utilization_ratio
aws_commitment_coverage_ratio
aws_commitment_unused_hours
aws_commitment_covered_spend_amount
aws_commitment_on_demand_cost_amount
aws_commitment_net_savings_amount
aws_cost_anomaly_active
aws_cost_anomaly_count
aws_cost_anomaly_impact_amount
aws_cost_anomaly_last_detected_timestamp_seconds
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
