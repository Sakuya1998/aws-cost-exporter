[English](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Dashboards-PromQL-and-Alerts) | [简体中文](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Dashboards-PromQL-and-Alerts-zh-CN)

# 仪表盘、PromQL 与告警

导入 `dashboards/grafana/aws-cost-exporter.json`，将 `DS_PROMETHEUS` 绑定到抓取 Exporter 的 Prometheus 数据源。内置 Dashboard 提供 `target`、provider、basis、currency 选择器，不依赖额外插件。

## 安全金额聚合

保留所有会计维度：

```promql
sum by (target, provider, cost_basis, currency) (
  aws_cost_month_to_date_amount
)
```

Service 分组：

```promql
topk(10,
  sum by (target, provider, cost_basis, currency, aws_service) (
    aws_cost_service_month_to_date_amount
  )
)
```

不要直接使用 `sum(aws_cost_month_to_date_amount)`，它会把不同账户、currency、provider 和不兼容 basis 混在一起。

## 预计月末成本

Forecast 包含今天，MTD 也已经包含今天的累计值，需要避免重复：

```promql
aws_cost_month_to_date_amount
- aws_cost_daily_amount
+ aws_cost_month_forecast_mean_amount
```

三个指标必须匹配同一组 `target`、`provider`、`cost_basis`、`currency`。

## 健康查询

```promql
aws_cost_exporter_collector_up == 0
```

```promql
time() - aws_cost_exporter_last_success_timestamp_seconds
```

```promql
increase(aws_cost_exporter_pagination_pages_total[1h]) > 100
```

## 内置告警

`rules/prometheus/aws-cost-exporter.rules.yaml` 包含 Collector 失败、Cache stale、分页突增、持续限流、overflow 和 shutdown timeout。告警分组和 annotation 应保留 `target`，金额告警还必须保留 `currency`、provider、basis。

AWS 成本数据可能延迟或 backfill。Daily gauge 在 Prometheus 中形成的是当前 UTC 账单日数值的 scrape 历史，不是 Cost Explorer 的历史每日明细表。
