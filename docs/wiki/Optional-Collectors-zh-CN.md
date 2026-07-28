[English](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Optional-Collectors) | [简体中文](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Optional-Collectors-zh-CN)

# 可选 Collectors

Organizations、Budgets、Commitments、Anomalies、Tags 和 CUR 默认关闭。它们不参与 `/ready`；只有 required target 上启用的 Cost Explorer collector 会影响 readiness。

## Organizations

通过 `ListAccounts` 和 `DescribeOrganization` 为 Linked Account 指标补充非敏感名称与状态。它不会创建 target，也不会导出账户 Email。

- `account_ids` 非空：只导出配置的 allowlist。
- `account_ids` 为空：只导出 account cost collector 已观察到的账户。
- observed-account 模式要求启用 account collector。

## Budgets

要求非空、唯一的精确 Budget name allowlist。只接受 `budget_type=COST`，因为 Usage/Utilization Budget 不使用 currency 单位。缺少 actual 或 forecast 值时省略相应 series，不伪造零值。

## Savings Plans 与 Reserved Instances

Commitment collector 查询 `savings_plan` 和 `reservation` 的利用率与覆盖率汇总，发布账户级 ratio、unused hours、covered spend、on-demand equivalent 和 net savings。Plan ID、Reservation ID、Service、Region 不作为 label。

## Cost Anomaly Detection

Anomaly collector 分页读取异常，并输出有界汇总：active、count、累计 impact、最近检测时间。不会暴露 Anomaly ID、根因、Service、Region 或原始 AWS 文本。

## Tag 成本

每个公开 Tag key 都必须在 allowlist 中，并配置独立 `max_values`。Cost Explorer 每个 key 单独查询；CUR 还要求在 `cur.tag_columns` 中映射唯一、安全的 SQL identifier。

超限值聚合到 `__other__`，金额保持守恒。启动校验会拒绝无法放入 `collection.tags.series_limit` 的最坏情况预算。重复 label set 或硬上限违规会拒绝整次刷新并保留旧数据。

## 失败行为

每个可选 collector 拥有独立 `CollectorID`、刷新周期、状态、single-flight、backoff 和上一次成功 partial snapshot。某个可选域授权失败或超时，不会阻塞其他 target 或 provider。
