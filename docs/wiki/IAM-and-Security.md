[English](https://github.com/Sakuya1998/aws-cost-exporter/wiki/IAM-and-Security) | [简体中文](https://github.com/Sakuya1998/aws-cost-exporter/wiki/IAM-and-Security-zh-CN)

# IAM and Security

Grant only the APIs required by enabled collectors. Keep source-principal permissions and target-role permissions separate.

## Collector permissions

- Cost totals, dimensions, and forecast: `ce:GetCostAndUsage`, `ce:GetCostForecast`.
- Organizations metadata: `organizations:ListAccounts`, `organizations:DescribeOrganization`.
- Budgets: `budgets:ViewBudget`.
- Savings Plans/RI summaries and anomalies: the read operations listed in `commitments-anomalies-readonly.json`.
- CUR: Athena start/status/results/stop, scoped Glue catalog reads, CUR input-prefix reads, and Athena result-prefix access.

STS `GetCallerIdentity` normally requires no explicit allow. A source principal needs `sts:AssumeRole` only for each exact configured role ARN. Do not grant wildcard role resources.

Use the policies in [`examples/iam`](https://github.com/Sakuya1998/aws-cost-exporter/tree/master/examples/iam) as templates and replace every placeholder with exact account, region, database, table, bucket, prefix, role, and principal values.

## Secret handling

- YAML stores environment-variable names, never access keys, secret keys, session tokens, or ExternalId values.
- Helm uses `config.secretEnvRefs` for existing Secrets and `awsSharedConfig.existingSecret` for mounted Profile files.
- Docker mounts `${HOME}/.aws` read-only or injects environment variables through the runtime.
- Logs, metrics, debug output, fixtures, issues, and screenshots must not contain credentials, account email, raw AWS responses, or private cost data.
- Protect optional debug endpoints with an authenticated proxy or NetworkPolicy.

## Container and Kubernetes posture

The published container runs non-root with a read-only-compatible filesystem model. Keep the chart at one replica, use a dedicated ServiceAccount, apply the bundled NetworkPolicy where suitable, and avoid mounting Secrets into paths used for logs or Athena results.

## Reporting vulnerabilities

Follow the repository [Security Policy](https://github.com/Sakuya1998/aws-cost-exporter/security/policy). Use GitHub's private “Report a vulnerability” flow. Do not put a vulnerability, credential, account identifier, or billing sample in a public issue.

Before v1.0, only the latest released minor version receives security fixes.
