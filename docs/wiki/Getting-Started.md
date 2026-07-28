[English](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Getting-Started) | [简体中文](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Getting-Started-zh-CN)

# Getting Started

This path starts one required Cost Explorer target with the AWS default credential chain.

## Prerequisites

- Go 1.24 or a published binary/container.
- Credentials for account `444455556666` available through the AWS default chain.
- `ce:GetCostAndUsage` and `ce:GetCostForecast` permissions.
- Network access to AWS APIs.

Copy `configs/aws-cost-exporter.example.yaml` and replace the placeholder account ID. Keep optional collectors disabled for the first run.

```yaml
aws:
  region: us-east-1
  credentials:
    sources:
      runtime:
        type: default_chain

targets:
  - name: payer-prod
    account_id: "444455556666"
    required: true
    credentials:
      source: runtime
    cost_explorer:
      enabled: true
      filters:
        linked_account_ids: []
        services: []
        regions: []
```

Validate before starting:

```bash
./aws-cost-exporter --config config.yaml --check-config
./aws-cost-exporter --config config.yaml
```

Check the endpoints:

```bash
curl http://localhost:8080/healthz
curl http://localhost:8080/ready
curl http://localhost:8080/metrics
curl http://localhost:8080/version
```

`/ready` can return 503 until every enabled Cost Explorer collector on the required target has completed its first successful refresh. Inspect `aws_cost_exporter_collector_up` and logs by `target` and `collector` before changing IAM or retry settings.

Next, add Profiles or AssumeRole targets, filters, and optional collectors one at a time. Re-run `--check-config` after every change.
