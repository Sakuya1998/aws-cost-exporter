[English](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Dashboards-PromQL-and-Alerts) | [简体中文](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Dashboards-PromQL-and-Alerts-zh-CN)

# Dashboards, PromQL and Alerts

Import `dashboards/grafana/aws-cost-exporter.json` and bind `DS_PROMETHEUS` to the Prometheus data source that scrapes the exporter. The bundled dashboard has `target`, provider, basis, and currency controls and no plugin dependency.

## Safe monetary aggregation

Preserve all accounting dimensions:

```promql
sum by (target, provider, cost_basis, currency) (
  aws_cost_month_to_date_amount
)
```

Service breakdown:

```promql
topk(10,
  sum by (target, provider, cost_basis, currency, aws_service) (
    aws_cost_service_month_to_date_amount
  )
)
```

Do not use an unqualified `sum(aws_cost_month_to_date_amount)`: it can merge accounts, currencies, providers, and incompatible bases.

## Estimated month end

Forecast includes today while MTD already contains today's accumulated value. Avoid double counting:

```promql
aws_cost_month_to_date_amount
- aws_cost_daily_amount
+ aws_cost_month_forecast_mean_amount
```

Match the same `target`, `provider`, `cost_basis`, and `currency` on all terms.

## Health queries

```promql
aws_cost_exporter_collector_up == 0
```

```promql
time() - aws_cost_exporter_last_success_timestamp_seconds
```

```promql
increase(aws_cost_exporter_pagination_pages_total[1h]) > 100
```

## Bundled alerts

`rules/prometheus/aws-cost-exporter.rules.yaml` includes collector failure, stale cache, pagination spike, sustained throttling, overflow, and shutdown-timeout signals. Keep `target` in alert grouping and annotations. Monetary alerts must keep `currency`, provider, and basis.

Billing data can arrive late and be backfilled. Daily gauges represent the current UTC billing-day value observed over Prometheus scrape history, not a historical Cost Explorer daily table.
