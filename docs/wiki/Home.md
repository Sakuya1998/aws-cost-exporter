[English](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Home) | [简体中文](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Home-zh-CN)

# AWS Cost Exporter Wiki

This Wiki documents the current stable release, **v1.0.0**.

v1.0 adds no collectors. It freezes the configuration, metrics, and HTTP
behavior as the v1 contract and adds machine-checked compatibility, lifecycle,
capacity, security, and release evidence. See the
[v1.0.0 verification record](https://github.com/Sakuya1998/aws-cost-exporter/blob/master/docs/releases/v1.0.0-verification.md)
for the verified artifacts and explicitly deferred operational exercises.

AWS Cost Exporter turns low-frequency AWS billing data into stable, target-scoped Prometheus metrics. It is an exporter, not a financial reconciliation system. AWS remains the source of truth.

## What it supports

- Explicit multi-account targets with default-chain, Profile, environment-backed, or AssumeRole credentials.
- Cost Explorer totals, service, region, linked-account, forecast, and allowlisted tag costs.
- Unblended, amortized, and net cost bases with explicit `provider` and `cost_basis` labels.
- CUR 2.0 aggregate queries through Athena.
- Organizations account metadata, COST Budgets, Savings Plans and Reserved Instances summaries, and Cost Anomaly Detection summaries.
- Prometheus rules, a Grafana dashboard, Docker images, a Helm OCI chart, and least-privilege IAM examples.

## Core runtime contract

Background collectors call AWS and Athena on bounded schedules. Successful partial snapshots are validated and atomically published to memory. `/metrics` only reads the immutable snapshot and never calls AWS or Athena.

A failed refresh keeps the last successful data for that target and collector. Other targets continue independently. `/healthz` reports process liveness; `/ready` requires fresh successful Cost Explorer data for required targets.

## Start here

1. Read [Getting Started](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Getting-Started).
2. Choose an [installation method](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Installation).
3. Review [credentials and target identity](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Credentials-and-Targets).
4. Validate configuration with `--check-config`.
5. Import the dashboard and alert rules after the first successful refresh.

Use one replica unless duplicate AWS and Athena requests are intentional. The
v1 contract keeps `replicaCount: 1`, memory-only snapshots, and no leader
election or shared cache.
