[English](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Configuration-Reference) | [简体中文](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Configuration-Reference-zh-CN)

# 配置参考

该精确 Schema 是稳定的 v1 契约。项目保证 v1.x 向后兼容：
patch 和 minor 版本不会删除或重新解释已接受的 Key、默认值、枚举值或校验行为。
minor 版本可以增加可选配置。Deprecated 项在删除前至少保留最近两个 minor 版本
和六个月，并且只能在未来 major 版本移除。系统不提供静默迁移 Parser。

本页描述 v1.0.0 的精确配置 schema。加载器使用 exact unmarshal：未知、拼写错误、已弃用和旧版字段都会被拒绝。

## 顶层键

```text
server
log
aws
targets
collection
cache
telemetry
```

不存在 `config_version` 或兼容解析器。

## 校验流程

```bash
./aws-cost-exporter --config config.yaml --check-config
```

生产启动使用完全相同的校验与环境变量解析。环境变量覆盖使用双下划线，例如 `AWS_COST_EXPORTER_SERVER__LISTEN_ADDRESS=:9090`。

## Server 与日志

`server` 配置监听地址、`/metrics` 路径、正数 HTTP timeout、关闭 timeout、1..1000 的 `max_in_flight` 和可选 debug 端点。Metrics、health、readiness、version、根路径以及启用的 debug 路径不能冲突。`log` 选择 `json`/`text`、级别和 source，日志始终写入 stdout/stderr。

## AWS

`aws` 配置 Cost Explorer region、命名凭证源、请求 timeout、STS/Cost Explorer/Organizations/Budgets/Athena 测试 endpoint、retry 和限流。

- 凭证源：1..20，名称唯一。
- `retry.max_attempts`：1..10。
- 全局与 target burst：1..5。
- RPS：有限正数，最大 10。
- Retry delay 与请求 timeout：正数且有上限。

## Targets

- Target 数量：1..20。
- 名称匹配 `^[a-z][a-z0-9_-]{0,31}$` 且唯一。
- Account ID 必须为唯一的 12 位数字。
- 至少一个 required target 启用 Cost Explorer。
- Role ARN 必须是目标账户中的精确 IAM Role ARN。
- Budget name、Organizations Account ID、Tag key 和 CUR tag column 必须唯一。

## Collection

`collection` 配置启动刷新、0..0.5 的有限 `jitter_ratio`、1..100 全局并发、受限 failure backoff 和各数据域刷新周期。

- Cost basis：`unblended`、`amortized`、`net` 的非空唯一子集。
- Cost Explorer 与 Organizations `series_limit` 最大 2000。
- Budgets `series_limit` 最大 500。
- `max_pages` 通常为 1..100；CUR 最大 200。
- Tag key 最多 20 个；每个 `max_values` 为 1..500。
- CUR 最大 100,000 行、1..10 个 `max_currencies`、20,000 series。
- `overflow_label` 必须已经 Trim，且不能与 provider/dimension 值冲突。

`targets[].cur.query_timeout` 必须大于 `aws.request_timeout`、不超过一小时并小于 `collection.cur.refresh_interval`；`poll_interval` 为 100ms..1m。`output_location` 必须包含合法小写 S3 Bucket 和非空 prefix。

## Cache 与 Telemetry

`cache.freshness_ttl`、`cache.stale_after` 必须为正数，stale 时间应大于正常刷新周期。Telemetry 控制标准 Go 与 process collector。

请从 [`configs/aws-cost-exporter.example.yaml`](https://github.com/Sakuya1998/aws-cost-exporter/blob/master/configs/aws-cost-exporter.example.yaml) 开始，不要把 Secret 写进 YAML。
