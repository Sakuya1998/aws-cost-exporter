[English](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Architecture) | [简体中文](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Architecture-zh-CN)

# 架构

## v1 运维边界

v1 在 2 vCPU、512 MiB 参考环境中支持单进程 20 个 target 和 20,000+
业务 Series。系统继续使用仅内存 Snapshot：进程重启会丢失缓存值，持久保留
由 Prometheus 负责。系统不实现 Leader Election、Fencing、共享持久化或多副本
刷新协调。Helm 替换策略防止新旧 Pod 同时产生付费请求。

AWS Cost Exporter 是依赖向内的 Modular Monolith。Domain package 不导入 AWS SDK、Prometheus、HTTP、Cobra 或 Viper。

## 模块

- `internal/domain`：Target Identity 与 immutable Cost、Budget、Commitment、Anomaly、Organization、Tag、Aggregate Snapshot。
- `internal/ports`：Collector 和 Application 消费的窄接口。
- `internal/aws`：SDK Client、凭证组合、AssumeRole、Limiter、Pagination、Mapping、安全错误分类。
- `internal/collector`：把 Reader Port 映射为 typed `PartialSnapshot`。
- `internal/scheduler`：刷新周期、target single-flight、target 串行执行、全局并发和受限 failure backoff。
- `internal/cache/memory`：copy-on-write partial map 和 atomic aggregate publication。
- `internal/metrics`：固定 Prometheus Descriptor 与 Snapshot 遍历。
- `internal/httpserver`：Metrics、Probe、Version、可选诊断。
- `internal/app`：Composition Root。

## 身份模型

```go
type TargetID string

type CollectorID struct {
    Target TargetID
    Name   string
}
```

每个 Domain Value、Cache Partial、Status、Scheduler Job、日志事件和 target 级指标都携带 Target Identity。Cost 还携带有界 Provider 与 Basis，防止 Cost Explorer/CUR 或不同会计视图被静默合并。

## 运行数据流

```text
基础 AWS Config
  -> Target Credential / AssumeRole Cache
  -> Target AWS Clients
  -> Typed Collectors
  -> Shared Scheduler
  -> Copy-on-write Cache Rebuild
  -> Atomic Immutable Snapshot
  -> Prometheus Collectors
  -> HTTP Server
```

Prometheus scrape 无应用锁读取一个 atomic pointer，并按值遍历，不复制完整 slice。Target/Collector 失败只更新自身状态并保留旧 partial。

## AWS Attempt 策略

```text
global limiter -> target limiter -> SDK attempt token -> HTTP request
```

Wrapper 安装在 SDK attempt-token 路径，因此初始请求和 retry 都经过限流，同时保留 SDK retry/backoff/token bucket。取消会传播到 wait、pagination、Athena polling、backoff、worker。

## 决策

- 拒绝 scrape 时查询 AWS，因为会把 Prometheus 与付费分页 API 耦合。
- 暂不使用本地持久缓存，因为 Prometheus 已负责保留，持久化会增加迁移、加密、HA 问题。
- 拒绝动态 Go Plugin，Collector 使用编译期注册和窄接口。
- v1.0.0 保持单副本，参见 [ADR 0002](https://github.com/Sakuya1998/aws-cost-exporter/blob/master/docs/adr/0002-ha-refresh-coordination.md)。

权威决策记录位于 [`docs/adr`](https://github.com/Sakuya1998/aws-cost-exporter/tree/master/docs/adr)。
