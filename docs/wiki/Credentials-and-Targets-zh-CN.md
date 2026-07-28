[English](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Credentials-and-Targets) | [简体中文](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Credentials-and-Targets-zh-CN)

# 凭证与 Targets

每个 AWS 账户都通过显式 `target` 配置。Target 包含稳定名称、声明的 12 位 Account ID、required/optional 状态、一个凭证源、可选 AssumeRole 和启用的 Collector。

## 凭证源

```yaml
aws:
  credentials:
    sources:
      runtime:
        type: default_chain
      account-a:
        type: profile
        profile: account-a
      environment-source:
        type: static_env
        access_key_id_env: EXPORTER_ACCESS_KEY_ID
        secret_access_key_env: EXPORTER_SECRET_ACCESS_KEY
        session_token_env: EXPORTER_SESSION_TOKEN
```

- `default_chain` 使用标准 AWS SDK 查找顺序，包括环境变量、shared files、容器凭证和实例元数据。
- `profile` 使用 AWS shared configuration，支持 `source_profile`、SSO、`credential_process` 和 Profile Role Chain。
- `static_env` 只保存环境变量名称；被引用变量在 `--check-config` 和启动时必须存在且非空。

同一个 source 可以支持多个 AssumeRole target，但最多只能有一个 direct target。Direct target 直接使用 source credentials。

## AssumeRole

```yaml
targets:
  - name: payer-prod
    account_id: "444455556666"
    required: true
    credentials:
      source: runtime
      assume_role:
        role_arn: arn:aws:iam::444455556666:role/aws-cost-exporter-reader
        external_id_env: EXPORTER_PAYER_EXTERNAL_ID
        session_name: aws-cost-exporter-payer-prod
```

Role ARN 必须精确、不能带 wildcard，并且账户与 `account_id` 一致。每个 target 使用独立 credentials cache。ExternalId 只从指定环境变量读取，不会进入 YAML、日志、指标或 debug 输出。

## 身份校验

Collector 调用计费 API 前，最终凭证会执行 STS `GetCallerIdentity`，返回账户必须等于配置的 `account_id`。校验按 target single-flight、经过限流，成功后缓存。错误的 Profile 或静态凭证不能把成本发布到错误 target。

授权失败属于运行期 target 故障，不会阻止其他 target 启动和刷新。

## 多账户模式

- 集中式部署优先使用一个 payer source 加多个精确 AssumeRole target。
- 工作站或已有 AWS shared configuration 适合使用独立 Profile。
- 环境变量静态凭证只作为兼容方案，通过 Secret 注入并在外部轮换。
- Organizations 元数据不会自动发现或创建 target。

使用 [`examples/iam`](https://github.com/Sakuya1998/aws-cost-exporter/tree/master/examples/iam) 中的精确策略，避免 wildcard `sts:AssumeRole` 权限。
