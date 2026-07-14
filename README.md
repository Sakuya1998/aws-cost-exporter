# AWS Cost Exporter

Repository: [github.com/sakuya1998/aws-cost-exporter](https://github.com/sakuya1998/aws-cost-exporter)

AWS Cost Exporter is a Prometheus exporter for AWS Cost Explorer: a "Node
Exporter for AWS cost" for teams that already operate Prometheus, Grafana, and
Alertmanager. It is not a financial reconciliation system, billing UI, or
invoice source. Unlike YACE it specializes in billing data; unlike OpenCost and
Kubecost it is not Kubernetes allocation software; unlike CUR it does not offer
line-item analytics. See [ARCHITECTURE.md](ARCHITECTURE.md).

## Data and cost warnings

Cost Explorer API requests are billed per paginated request. Defaults refresh
every 6h and rate-limit AWS calls to 0.5 RPS.
The exporter does not call AWS during a Prometheus scrape; collectors refresh
an atomic in-memory snapshot.
AWS updates billing data only periodically, and delayed charges can be
backfilled. Metrics are operational observations, not accounting records.
Never sum different `currency` label values.

## Prerequisites and IAM

Enable Cost Explorer in the payer account, then attach
[`examples/iam/mvp-readonly.json`](examples/iam/mvp-readonly.json). MVP requires
only `ce:GetCostAndUsage` and `ce:GetCostForecast` on `Resource: "*"`;
`ce:GetDimensionValues`, Organizations access, write access, and static keys are
not required. A management account can group visible costs by `LINKED_ACCOUNT`
without calling Organizations APIs.

Credentials come from the AWS default chain: environment, shared profile,
container credentials, IRSA, or EKS Pod Identity. There are no access-key
configuration fields. Files under `examples/iam` for Organizations and
AssumeRole document future capabilities and are not implemented in v0.1.
AssumeRole examples use an explicit role ARN and ExternalId; attach the MVP
policy to the target reader role.

## Install from a release

Published artifacts are attached to GitHub Releases. Container images and Helm
charts are also published to GHCR under `sakuya1998`.

```sh
docker pull ghcr.io/sakuya1998/aws-cost-exporter:0.1.0
helm install aws-cost-exporter oci://ghcr.io/sakuya1998/charts/aws-cost-exporter --version 0.1.0
```

Verify signatures with `cosign verify` against the release digest before
deploying in production. Stable tags also publish `latest`; pre-releases do not.

## Run a binary

Clone the repository and build locally, or install a tagged release binary from
GitHub Releases.

```sh
git clone https://github.com/sakuya1998/aws-cost-exporter.git
cd aws-cost-exporter
```

Go module: `github.com/sakuya1998/aws-cost-exporter`. Go 1.24 or newer is
required.

```sh
make build
./aws-cost-exporter --config configs/aws-cost-exporter.example.yaml --check-config
./aws-cost-exporter --config configs/aws-cost-exporter.example.yaml
```

Useful flags are `--config`, `--listen-address`, `--log-level`,
`--check-config`, and `--version`. Configuration precedence is flags,
`AWS_COST_EXPORTER_` environment variables, YAML, then defaults. The complete
environment name uses `__` between nested keys, for example
`AWS_COST_EXPORTER_SERVER__LISTEN_ADDRESS=:9090`. The complete schema is in
`configs/aws-cost-exporter.example.yaml` and
`internal/config/schema.go`; configuration reload requires a restart.

## Run with Docker Compose

`docker-compose.yml` mounts the example config and host AWS profile read-only.
Set `AWS_PROFILE`; on Linux set `AWS_HOST_UID` and `AWS_HOST_GID` when profile
files are not readable by UID 65532. On Windows, set `HOME` to the profile home
before Compose resolves `${HOME}/.aws`.

```sh
docker compose up --build
curl http://localhost:8080/ready
```

The image is distroless, non-root, and compatible with a read-only root
filesystem. It contains no shell. See
[logging operations](docs/operations/logging.md) for external rotation.

## Install with Helm

Use IRSA or EKS Pod Identity; the chart does not create AWS credential Secrets.
Install from the local chart path during development, or from GHCR after a
release:

```sh
helm install aws-cost-exporter ./charts/aws-cost-exporter \
  --set-string serviceAccount.annotations.eks\\.amazonaws\\.com/role-arn=arn:aws:iam::111122223333:role/aws-cost-exporter

helm install aws-cost-exporter oci://ghcr.io/sakuya1998/charts/aws-cost-exporter \
  --version 0.1.0 \
  --set-string serviceAccount.annotations.eks\\.amazonaws\\.com/role-arn=arn:aws:iam::111122223333:role/aws-cost-exporter
```

The default image repository is `ghcr.io/sakuya1998/aws-cost-exporter`.

The chart defaults to one replica. More replicas multiply Cost Explorer calls
and expose duplicate scrape targets. Optional integrations include
`serviceMonitor.enabled`, `prometheusRule.enabled`, `networkPolicy.enabled`,
and `podDisruptionBudget.enabled`.

## Prometheus and HTTP

Scrape `GET /metrics` on port 8080 by default; `server.metrics_path` can change
the path. `GET /healthz` reports process liveness; `GET /ready` requires initial
data and rejects stale snapshots; `GET /version` returns build metadata. Debug
and pprof routes default to 404 and must be network-restricted when enabled.

Business gauges are:

```text
aws_cost_daily_amount, aws_cost_month_to_date_amount
aws_cost_service_daily_amount, aws_cost_service_month_to_date_amount
aws_cost_region_daily_amount, aws_cost_region_month_to_date_amount
aws_cost_account_daily_amount, aws_cost_account_month_to_date_amount
aws_cost_month_forecast_mean_amount, aws_cost_month_forecast_lower_bound_amount
aws_cost_month_forecast_upper_bound_amount
```

Every business series has `currency`. Empty regions become `global`; each
dimension family is capped at `series_limit` (default 1000), with overflow
conserved in `__other__`. Families never form a service/region/account Cartesian
product. There is no date label: daily charts use scrape history.

Self-observability metrics are:

```text
aws_cost_exporter_collector_up, aws_cost_exporter_cache_age_seconds
aws_cost_exporter_last_success_timestamp_seconds, aws_cost_exporter_last_attempt_timestamp_seconds
aws_cost_exporter_snapshot_series, aws_cost_exporter_refresh_total
aws_cost_exporter_aws_api_requests_total, aws_cost_exporter_aws_api_retries_total
aws_cost_exporter_scheduler_skipped_runs_total
aws_cost_exporter_dimension_overflow_values_total
aws_cost_exporter_refresh_duration_seconds, aws_cost_exporter_aws_api_request_duration_seconds
aws_cost_exporter_build_info
```

`collector_up` describes the latest attempt; stale data is indicated by cache
age and readiness. Old snapshots remain available from `/metrics`.

## Operations assets

- Dashboard: `dashboards/grafana/aws-cost-exporter.json`
- Rules: `rules/prometheus/aws-cost-exporter.rules.yaml`
- Troubleshooting: `docs/operations/troubleshooting.md`
- Security reporting: [SECURITY.md](SECURITY.md)
- Contribution guide: [CONTRIBUTING.md](CONTRIBUTING.md)
