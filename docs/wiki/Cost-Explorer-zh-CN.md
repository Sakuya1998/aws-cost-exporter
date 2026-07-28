[English](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Cost-Explorer) | [简体中文](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Cost-Explorer-zh-CN)

# Cost Explorer

Cost Explorer 是默认 Provider，使用 `provider="cost_explorer"`。它由后台 Job 查询当前 UTC 账单日和月累计窗口。

## Collectors

- `total`：当日与月累计总成本。
- `service`：按 AWS Service 分组。
- `region`：按 AWS Region 分组。
- `account`：按 Linked Account 分组。
- `forecast`：从今天到当前月末的均值和预测区间。
- Tag 成本：每个 allowlist key 单独执行一组查询。

Linked Account ID、Service、Region 过滤器按 target 配置。空过滤器允许 AWS 返回所有匹配值，可能增加付费分页。

## Cost Basis

`collection.cost_explorer.cost_bases` 是以下值的非空唯一子集：

- `unblended`
- `amortized`
- `net`

每条成本指标都有 `target`、`provider`、`cost_basis`、`currency`。不同 basis/provider 是不同会计视图，不能直接相加。Forecast 使用第一个配置的 basis。

## 请求、分页与重试

启用 total、service、region、account 时，每个 basis 的一次无分页刷新会执行八个 `GetCostAndUsage` 逻辑操作：四个 Collector 分别查询 daily 和 month-to-date。Forecast 增加 `GetCostForecast`，每个 Cost Explorer Tag key 每个 basis 增加两个操作。

当 target 数为 `T`、basis 数为 `B`、Tag key 数为 `K`、平均页数为 `P`：

```text
每次刷新近似请求数 = T * (((8 + 2*K) * B * P) + forecast + commitment/anomaly operations)
```

AWS 当前每个可计费 Cost Explorer 请求约 USD 0.01，部署前应确认最新价格。SDK retry 可能产生额外计费 HTTP attempt。

初始 attempt 和 retry 都依次获取全局 limiter、target limiter，同时保留 AWS SDK retry/backoff/token bucket。逻辑请求、成功分页和授权 retry 是运维计数，不等同于 AWS 账单。

## 基数控制

Service、Region、Account 受 `series_limit` 限制，超限值聚合到通常为 `__other__` 的 `overflow_label`，金额保持守恒。Tag 值使用每个 key 的 `max_values` 和相同的金额守恒规则。
