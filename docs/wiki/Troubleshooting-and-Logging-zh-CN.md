[English](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Troubleshooting-and-Logging) | [简体中文](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Troubleshooting-and-Logging-zh-CN)

# 故障排查与日志

## `/ready` 返回 503

`missing` 表示 required target 上某个启用的 Cost Explorer collector 从未成功；`stale` 表示最近成功已超过 `cache.stale_after`。按 `target`、`collector` 检查 `aws_cost_exporter_collector_up`、`aws_cost_exporter_cache_age_seconds` 和最近成功时间。

可选 Collector 不参与 readiness。失败 Collector 会保留旧数据，因此 `/metrics` 返回 200 不代表数据新鲜。

## AWS 权限与身份

Cost Explorer 403 时检查 `ce:GetCostAndUsage`、`ce:GetCostForecast`，然后确认最终凭证账户等于 `target.account_id`；每个 target 在计费调用前执行 STS `GetCallerIdentity`。

Profile 失败时检查 shared AWS files 和无界面 SSO 登录；AssumeRole 检查精确 Role ARN、Source Principal、`sts:ExternalId` 和 `external_id_env`。敏感值和 Caller ARN 不会出现在日志中。

## 限流与请求成本

出现 throttling 时检查 `aws_cost_exporter_aws_api_requests_total`、`aws_cost_exporter_aws_api_retries_total`、`aws_cost_exporter_pagination_pages_total`。SDK retry 在全局与 target limiter 后执行。提高 RPS 前先收紧过滤、页数、basis 和刷新频率。

Billing 数据可能延迟或 backfill。Forecast 覆盖 today through month end，估算月末时需要从 MTD 中减去今日 daily 再加 forecast。

## 基数

超限值聚合到 `__other__`；检查 `aws_cost_exporter_dimension_overflow_values_total`。`overflow_label` 不能与 provider 或真实 dimension 值冲突。不同 `currency`、provider、basis 不能合并。提高 `max_currencies` 前同时提高兼容的 CUR 与 Tag series budget。

## CUR 与 Athena

检查 CUR Region、Workgroup、Glue Table、输入 Prefix、结果 Prefix、`query_timeout`、`poll_interval`、行/页上限和 `athena:StopQueryExecution`。检查 `StartQueryExecution`、`GetQueryExecution`、`GetQueryResults`、`StopQueryExecution` 操作。失败或异常结果会保留旧数据。

## Shutdown、副本与 Debug

关闭超过配置 timeout 时检查 `aws_cost_exporter_scheduler_shutdown_timeouts_total`。除非接受重复请求，否则保持一个 replica。Debug 默认关闭，启用时应由 NetworkPolicy 或认证代理保护。

## 日志与轮转

Exporter 将结构化 JSON 或 Text 日志写到 stdout/stderr，不创建、重新打开、轮转、压缩或删除文件。

- Docker：为 `local` 或 `json-file` Driver 配置大小和数量上限。
- Kubernetes：配置 kubelet 轮转并使用集群日志 Agent 转发。
- systemd：使用 journald 和中心化保留限制。
- 文件重定向：无法避免时才使用外部 logrotate。

成本元数据可能暴露账户结构，应限制日志访问。禁止记录凭证、Authorization Header、ExternalId、账户 Email 或原始 AWS Response。
