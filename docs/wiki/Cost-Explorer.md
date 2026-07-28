[English](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Cost-Explorer) | [简体中文](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Cost-Explorer-zh-CN)

# Cost Explorer

Cost Explorer is the default provider and uses `provider="cost_explorer"`. It queries current UTC billing-day and month-to-date windows in background jobs.

## Collectors

- `total`: daily and month-to-date totals.
- `service`: grouped by AWS service.
- `region`: grouped by AWS region.
- `account`: grouped by linked account.
- `forecast`: mean and configured prediction bounds for the remaining current month, including today.
- tag costs: one query set per allowlisted key.

Filters for linked account IDs, services, and regions are configured per target. Empty filters permit AWS to return all matching values and can increase paid pagination.

## Cost bases

`collection.cost_explorer.cost_bases` is a non-empty unique subset of:

- `unblended`
- `amortized`
- `net`

Every cost series carries `target`, `provider`, `cost_basis`, and `currency`. Do not add different bases or providers; they represent alternative accounting views, not components of one total. Forecast uses the first configured basis.

## Requests, pages, and retries

With total, service, region, and account enabled, one unpaginated refresh performs eight `GetCostAndUsage` logical operations per basis: daily and month-to-date for four collectors. Forecast adds `GetCostForecast`. Every Cost Explorer tag key adds two operations per basis.

For `T` targets, `B` bases, `K` tag keys, and `P` average pages:

```text
approximate requests per refresh = T * (((8 + 2*K) * B * P) + forecast + commitment/anomaly operations)
```

AWS currently charges approximately USD 0.01 per billable Cost Explorer request; verify current AWS pricing before deployment. SDK retries can create extra billable HTTP attempts.

Each initial SDK attempt and retry acquires the global limiter, then the target limiter, while preserving AWS SDK retry/backoff/token-bucket behavior. `aws_cost_exporter_aws_api_requests_total`, pagination pages, and authorized retries are useful operational counts but are not the AWS invoice.

## Cardinality

Service, region, and account values are limited by `series_limit`. Excess values aggregate into `overflow_label`, normally `__other__`, while preserving monetary totals. Tag values use per-key `max_values` and the same conservation rule.
