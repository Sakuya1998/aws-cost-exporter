# Roadmap

This roadmap communicates direction rather than a delivery guarantee. Priorities
may change based on user evidence, AWS API behavior, security findings, and
maintainer capacity. Each release must preserve the exporter model described in
[ARCHITECTURE.md](ARCHITECTURE.md).

## v0.1: Cost Explorer exporter

Goal: deliver a reliable single-credential Cost Explorer exporter.
- Export current daily and month-to-date `UnblendedCost`.
- Export separate total, service, region, and linked-account metric families.
- Export month-end forecast mean and prediction bounds.
- Refresh asynchronously with bounded retries, rate limits, and atomic caching.
- Expose Prometheus metrics, health, readiness, version, and optional debug
  endpoints.
- Provide least-privilege IAM examples, Docker image, Helm chart, Grafana
  dashboard, alert rules, and operator documentation.

Exit criteria include stable metric names, no AWS calls on the scrape path,
bounded cardinality, race-free tests, integration coverage, signed release
artifacts, and documented Cost Explorer API cost and data-latency semantics.

## v0.2: Multi-account operation

Goal: operate safely across explicit AWS account boundaries.
- Discover optional account metadata through AWS Organizations.
- Support multiple explicitly configured targets using AssumeRole and ExternalId.
- Add AWS Budgets metrics without conflating budgets with observed cost.
- Introduce configuration migration and deprecation mechanics.
- Evaluate leader election or shared refresh coordination for high availability.

Multi-account support must retain target-level failure isolation and must not
require wildcard `sts:AssumeRole` permissions.

## v0.3: Commitment and detailed billing data

Goal: extend cost semantics while keeping metric contracts explicit.
- Add Savings Plans and Reserved Instance utilization and coverage.
- Add Cost Anomaly Detection signals.
- Add optional amortized and net cost bases with compatibility-safe metrics.
- Add a CUR 2.0 and Athena provider for billing-detail use cases.
- Add allowlisted tag-cost metrics with enforced cardinality budgets.

Provider-specific precision and freshness must be visible to users. Cost
Explorer and CUR values must not be silently merged when their semantics differ.

## v1.0: Stable operational contract

Goal: publish a production-stable exporter API.
- Guarantee semantic-versioning rules for metrics, labels, configuration, and
  HTTP endpoints.
- Publish upgrade, deprecation, backup, scaling, security, and SLO guidance.
- Validate high-availability behavior and large-organization performance.
- Reach at least 85 percent coverage in core domain and scheduling packages.
- Complete a public threat model and supply-chain release audit.

CloudFront distribution and S3 bucket cost metrics will be considered only when
CUR resource-level data can attribute them accurately. The project will not
infer resource costs from unrelated operational metrics.

## Continuing non-goals

- A billing dashboard or replacement for the AWS Billing console.
- Financial reconciliation, invoicing, or tax calculations.
- Kubernetes pod, namespace, or workload cost allocation.
- Automatic rightsizing, purchasing, or remediation.
- Unbounded AWS tags, resource IDs, dates, or error messages as metric labels.
