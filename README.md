# AWS Cost Exporter

[github.com/sakuya1998/aws-cost-exporter](https://github.com/sakuya1998/aws-cost-exporter) exports cached AWS billing data as target-scoped Prometheus metrics. It is an exporter, not a financial reconciliation system.

The current stable release is **v0.3.0**. It supports explicit multi-account targets, Cost Explorer, CUR 2.0 through Athena, Organizations, Budgets, Savings Plans and Reserved Instances summaries, Cost Anomaly Detection, and bounded tag costs. Prometheus does not call AWS during a Prometheus scrape; `/metrics` reads an immutable in-memory snapshot.

## Quick start

Build locally:

```bash
make build
./aws-cost-exporter --config configs/aws-cost-exporter.example.yaml
```

Container:

```bash
docker pull ghcr.io/sakuya1998/aws-cost-exporter:0.3.0
docker compose up --build
```

Helm OCI chart:

```bash
helm install aws-cost-exporter \
  oci://ghcr.io/sakuya1998/charts/aws-cost-exporter \
  --version 0.3.0 \
  --set config.data.targets[0].account_id=444455556666
```

Validate the exact production configuration and referenced environment variables before startup:

```bash
./aws-cost-exporter --config config.yaml --check-config
```

## HTTP endpoints

```text
/metrics   cached Prometheus metrics
/healthz   process liveness
/ready     required Cost Explorer snapshot readiness
/version   build metadata
```

## Documentation

- [English Wiki](https://github.com/Sakuya1998/aws-cost-exporter/wiki)
- [简体中文 Wiki](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Home-zh-CN)
- [Example configuration](configs/aws-cost-exporter.example.yaml)
- [IAM examples](examples/iam)
- [Grafana dashboard](dashboards/grafana/aws-cost-exporter.json)
- [Prometheus rules](rules/prometheus/aws-cost-exporter.rules.yaml)
- [Roadmap](ROADMAP.md)
- [Contributing](CONTRIBUTING.md)
- [Security policy](SECURITY.md)

The Wiki is generated from `docs/wiki` on `master`. Submit documentation corrections through a pull request rather than editing the GitHub Wiki directly.

## Development

```bash
go test ./...
go test -race ./...
go vet ./...
golangci-lint run
```

Licensed under the [Apache License 2.0](LICENSE).
