[English](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Metrics-Reference) | [简体中文](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Metrics-Reference-zh-CN)

# Metrics Reference

All business and target-scoped runtime metrics include `target`. Monetary series must never be aggregated across `currency`, `provider`, or `cost_basis` unless the query intentionally compares accounting views.

## Cost metrics

| Metric | Type | Additional labels | Meaning |
| --- | --- | --- | --- |
| `aws_cost_daily_amount` | gauge | `provider,cost_basis,currency` | Current UTC billing-day total |
| `aws_cost_month_to_date_amount` | gauge | `provider,cost_basis,currency` | Current UTC month-to-date total |
| `aws_cost_service_daily_amount` | gauge | `provider,cost_basis,currency,aws_service` | Daily cost by service |
| `aws_cost_service_month_to_date_amount` | gauge | `provider,cost_basis,currency,aws_service` | MTD cost by service |
| `aws_cost_region_daily_amount` | gauge | `provider,cost_basis,currency,aws_region` | Daily cost by region |
| `aws_cost_region_month_to_date_amount` | gauge | `provider,cost_basis,currency,aws_region` | MTD cost by region |
| `aws_cost_account_daily_amount` | gauge | `provider,cost_basis,currency,linked_account_id` | Daily cost by linked account |
| `aws_cost_account_month_to_date_amount` | gauge | `provider,cost_basis,currency,linked_account_id` | MTD cost by linked account |
| `aws_cost_month_forecast_mean_amount` | gauge | `provider,cost_basis,currency` | Remaining-month forecast mean, including today |
| `aws_cost_month_forecast_lower_bound_amount` | gauge | `provider,cost_basis,currency` | Remaining-month lower bound |
| `aws_cost_month_forecast_upper_bound_amount` | gauge | `provider,cost_basis,currency` | Remaining-month upper bound |
| `aws_cost_account_info` | gauge | `linked_account_id,account_name,account_status` | Non-sensitive Organizations metadata, value 1 |
| `aws_cost_tag_daily_amount` | gauge | `provider,cost_basis,currency,tag_key,tag_value` | Daily allowlisted tag cost |
| `aws_cost_tag_month_to_date_amount` | gauge | `provider,cost_basis,currency,tag_key,tag_value` | MTD allowlisted tag cost |

## Commitment and anomaly metrics

| Metric | Type | Labels |
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

`commitment_type` is bounded to `savings_plan` and `reservation`. No plan, reservation, or anomaly identifier is exposed.

## Budget metrics

All budget metrics use `target,budget_name,currency,budget_type,time_unit`:

- `aws_budget_limit_amount`
- `aws_budget_actual_amount`
- `aws_budget_forecasted_amount`

Only COST budgets are accepted. Missing actual or forecast data omits that series.

## Exporter metrics

| Metric | Type | Primary labels |
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
| `aws_cost_exporter_scheduler_shutdown_timeouts_total` | counter | process-global |

`operation`, `status`, `reason`, `collector`, provider, basis, and commitment labels use bounded enums. Raw AWS errors and request IDs cannot become labels.

The logical request, successful page, and retry counters are related to AWS activity but are not interchangeable with AWS-billed HTTP attempts.
