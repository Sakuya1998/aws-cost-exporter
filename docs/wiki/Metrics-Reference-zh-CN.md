[English](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Metrics-Reference) | [简体中文](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Metrics-Reference-zh-CN)

# 指标参考

所有业务指标和 target 级运行指标都包含 `target`。金额指标不能跨 `currency`、`provider`、`cost_basis` 聚合，除非查询明确是在比较会计视图。

## 成本指标

| Metric | 类型 | 额外 Labels | 含义 |
| --- | --- | --- | --- |
| `aws_cost_daily_amount` | gauge | `provider,cost_basis,currency` | 当前 UTC 账单日总成本 |
| `aws_cost_month_to_date_amount` | gauge | `provider,cost_basis,currency` | 当前 UTC 月累计成本 |
| `aws_cost_service_daily_amount` | gauge | `provider,cost_basis,currency,aws_service` | Service 当日成本 |
| `aws_cost_service_month_to_date_amount` | gauge | `provider,cost_basis,currency,aws_service` | Service 月累计成本 |
| `aws_cost_region_daily_amount` | gauge | `provider,cost_basis,currency,aws_region` | Region 当日成本 |
| `aws_cost_region_month_to_date_amount` | gauge | `provider,cost_basis,currency,aws_region` | Region 月累计成本 |
| `aws_cost_account_daily_amount` | gauge | `provider,cost_basis,currency,linked_account_id` | Linked Account 当日成本 |
| `aws_cost_account_month_to_date_amount` | gauge | `provider,cost_basis,currency,linked_account_id` | Linked Account 月累计成本 |
| `aws_cost_month_forecast_mean_amount` | gauge | `provider,cost_basis,currency` | 包含今天的剩余月度预测均值 |
| `aws_cost_month_forecast_lower_bound_amount` | gauge | `provider,cost_basis,currency` | 预测下界 |
| `aws_cost_month_forecast_upper_bound_amount` | gauge | `provider,cost_basis,currency` | 预测上界 |
| `aws_cost_account_info` | gauge | `linked_account_id,account_name,account_status` | 非敏感 Organizations 元数据，值为 1 |
| `aws_cost_tag_daily_amount` | gauge | `provider,cost_basis,currency,tag_key,tag_value` | allowlist Tag 当日成本 |
| `aws_cost_tag_month_to_date_amount` | gauge | `provider,cost_basis,currency,tag_key,tag_value` | allowlist Tag 月累计成本 |

## Commitment 与 Anomaly

| Metric | 类型 | Labels |
| --- | --- | --- |
| `aws_commitment_utilization_ratio` | gauge | `target,commitment_type,time_unit` |
| `aws_commitment_coverage_ratio` | gauge | `target,commitment_type,time_unit` |
| `aws_commitment_unused_hours` | gauge | `target,commitment_type,time_unit` |
| `aws_commitment_covered_spend_amount` | gauge | `target,commitment_type,time_unit,currency` |
| `aws_commitment_on_demand_cost_amount` | gauge | `target,commitment_type,time_unit,currency` |
| `aws_commitment_net_savings_amount` | gauge | `target,commitment_type,time_unit,currency` |
| `aws_cost_anomaly_active` | gauge | `target` |
| `aws_cost_anomaly_count` | gauge | `target` |
| `aws_cost_anomaly_impact_amount` | gauge | `target,currency` |
| `aws_cost_anomaly_last_detected_timestamp_seconds` | gauge | `target` |

`commitment_type` 只允许 `savings_plan`、`reservation`，不暴露 Plan、Reservation 或 Anomaly ID。

## Budget 指标

全部使用 `target,budget_name,currency,budget_type,time_unit`：

- `aws_budget_limit_amount`
- `aws_budget_actual_amount`
- `aws_budget_forecasted_amount`

只接受 COST Budget；缺失 actual/forecast 时省略相应 series。

## Exporter 指标

| Metric | 类型 | 主要 Labels |
| --- | --- | --- |
| `aws_cost_exporter_collector_up` | gauge | `target,collector` |
| `aws_cost_exporter_last_success_timestamp_seconds` | gauge | `target,collector` |
| `aws_cost_exporter_last_attempt_timestamp_seconds` | gauge | `target,collector` |
| `aws_cost_exporter_cache_age_seconds` | gauge | `target,collector` |
| `aws_cost_exporter_snapshot_series` | gauge | `target,collector` |
| `aws_cost_exporter_refresh_total` | counter | `target,collector,status` |
| `aws_cost_exporter_refresh_duration_seconds` | histogram | `target,collector` |
| `aws_cost_exporter_aws_api_requests_total` | counter | `target,operation,status` |
| `aws_cost_exporter_aws_api_retries_total` | counter | `target,operation,reason` |
| `aws_cost_exporter_aws_api_request_duration_seconds` | histogram | `target,operation` |
| `aws_cost_exporter_scheduler_skipped_runs_total` | counter | `target,collector,reason` |
| `aws_cost_exporter_dimension_overflow_values_total` | counter | `target,dimension` |
| `aws_cost_exporter_pagination_pages_total` | counter | `target,operation` |
| `aws_cost_exporter_cache_publish_errors_total` | counter | `target,collector,operation` |
| `aws_cost_exporter_build_info` | gauge | `version,revision,go_version` |
| `aws_cost_exporter_scheduler_shutdown_timeouts_total` | counter | 进程全局 |

`operation`、`status`、`reason`、`collector`、provider、basis、commitment label 都是有界枚举，原始 AWS 错误和 Request ID 不会成为 label。

逻辑请求、成功分页、retry 计数与 AWS 活动相关，但不等同于 AWS 实际计费 HTTP attempt。
