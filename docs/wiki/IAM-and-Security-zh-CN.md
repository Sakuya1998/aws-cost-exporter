[English](https://github.com/Sakuya1998/aws-cost-exporter/wiki/IAM-and-Security) | [简体中文](https://github.com/Sakuya1998/aws-cost-exporter/wiki/IAM-and-Security-zh-CN)

# IAM 与安全

只授予已启用 Collector 所需 API，并分离 Source Principal 与 Target Role 权限。

## Collector 权限

- 成本总计、维度和预测：`ce:GetCostAndUsage`、`ce:GetCostForecast`。
- Organizations 元数据：`organizations:ListAccounts`、`organizations:DescribeOrganization`。
- Budgets：`budgets:ViewBudget`。
- Savings Plans/RI 汇总与 Anomaly：`commitments-anomalies-readonly.json` 中的只读操作。
- CUR：Athena start/status/results/stop、限定 Glue Catalog 读取、CUR 输入 Prefix 读取和 Athena 结果 Prefix 访问。

STS `GetCallerIdentity` 通常不需要显式 Allow。Source Principal 只应对每个精确配置的 Role ARN 拥有 `sts:AssumeRole`，不要使用 wildcard Role Resource。

以 [`examples/iam`](https://github.com/Sakuya1998/aws-cost-exporter/tree/master/examples/iam) 为模板，将每个 Account、Region、Database、Table、Bucket、Prefix、Role、Principal 占位值替换为精确值。

## Secret 管理

- YAML 只保存环境变量名称，不保存 Access Key、Secret Key、Session Token 或 ExternalId 值。
- Helm 使用 `config.secretEnvRefs` 引用已有 Secret，使用 `awsSharedConfig.existingSecret` 挂载 Profile 文件。
- Docker 只读挂载 `${HOME}/.aws`，或通过 Runtime 注入环境变量。
- 日志、指标、Debug、Fixture、Issue、截图不能包含凭证、账户 Email、原始 AWS Response 或真实私有成本。
- 可选 Debug Endpoint 应由认证代理或 NetworkPolicy 保护。

## 容器与 Kubernetes

发布镜像以 non-root 方式运行，并兼容只读文件系统。Chart 保持一个副本，使用专用 ServiceAccount，适用时启用内置 NetworkPolicy，不要把 Secret 挂载到日志或 Athena 结果路径。

## 报告漏洞

遵循仓库 [Security Policy](https://github.com/Sakuya1998/aws-cost-exporter/security/policy)，使用 GitHub 私有 “Report a vulnerability”。不要在公开 Issue 中发布漏洞细节、凭证、Account ID 或账单样本。

v1.0 之前只为最新发布的 minor 版本提供安全修复。
