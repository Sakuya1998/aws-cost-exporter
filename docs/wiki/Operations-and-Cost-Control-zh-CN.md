[English](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Operations-and-Cost-Control) | [简体中文](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Operations-and-Cost-Control-zh-CN)

# 运行与成本控制

## v1 容量与 SLO

支持的参考负载是在 2 vCPU、512 MiB 上运行 20 个 target 和至少 20,000
条业务 Series。手工 24 小时稳定性运行要求 `/metrics` p99 小于 5 秒，成功率
至少 99.9%。AWS 与 Athena 延迟属于上游排除项，因为 Scrape 只读取缓存。
Release 报告还会判断 Heap、RSS、Goroutine 和连接是否持续增长。

## 升级、关闭与备份

Chart 固定 `replicaCount: 1`、`maxSurge: 0` 和 `maxUnavailable: 1`。
升级优先控制成本：不允许新旧 Pod 重叠产生付费请求，并接受新 Pod 刷新且 Ready
之前的临时指标中断。关闭时先让 `/ready` 以 `shutting_down` 变为不可用，再取消
Collector 并 Drain HTTP。

系统使用仅内存 Snapshot，不把它作为备份。应通过部署平台备份已评审配置和 Secret
引用，并保护 Prometheus 存储中的历史指标。Rollback 可以恢复旧 Binary 和配置，
但不能恢复已经失效的进程内 Snapshot。

## Probe 与 Snapshot

- `/healthz` 只表示进程存活。
- `/ready` 要求所有 required target 上启用的 Cost Explorer collector 都有成功且新鲜的 snapshot。
- Optional target、Organizations、Budgets、Commitments、Anomalies、Tags、CUR 不参与 readiness。
- `/metrics` 只返回 immutable snapshot，不会调用 AWS 或 Athena。

刷新失败会记录状态并保留 last successful snapshot。应结合 `collector_up`、最近成功时间和 cache age 判断；失败后旧值仍可能继续暴露。

## 调度与故障隔离

Collector 拥有独立刷新周期、single-flight、受限 failure backoff 和 `CollectorID`。同一 target 的 collector 串行执行，不能占满所有全局 slot；进程级 semaphore 执行 `collection.max_concurrency`。

Context 取消会停止 limiter wait、AWS 请求、Athena polling、SDK retry、backoff timer 和 worker。如果关闭超过 `server.shutdown_timeout`，检查 `aws_cost_exporter_scheduler_shutdown_timeouts_total`。

## 副本策略

Helm 保持 `replicaCount: 1`。v0.3.0 没有 Leader Election、共享 Cache 或刷新所有权。多个副本会重复付费请求，并成为重复 Prometheus scrape target。

## Cost Explorer 成本

AWS 当前每个可计费 Cost Explorer 请求约 USD 0.01，分页和 SDK retry 会增加成本。逻辑请求、retry、成功页数用于监控，最终以 AWS Billing 为准。

提高 RPS 或 page limit 前依次考虑：

1. 增加 target filter。
2. 关闭不用的 collector 或 basis。
3. 增加刷新周期。
4. 减少 Tag key 与 value budget。
5. 检查限流和分页告警。

## Athena 成本

Athena 按扫描字节计费。使用 CUR partition、专用 Workgroup、扫描上限、限定结果 prefix 和默认 24 小时周期。Totals 与 Tag 是独立查询，但每个固定查询只引用一次 CUR 表。Prometheus scrape 频率不会改变 Athena 查询频率。

## 新鲜度

Cost Explorer 与 CUR 的更新节奏不同。`cache.freshness_ttl` 表示预期新鲜度，`cache.stale_after` 控制 readiness stale。不要通过求和隐藏 provider 差异。
