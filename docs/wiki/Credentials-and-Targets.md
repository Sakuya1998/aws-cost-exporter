[English](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Credentials-and-Targets) | [简体中文](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Credentials-and-Targets-zh-CN)

# Credentials and Targets

Every AWS account is an explicit `target`. A target has a stable name, a declared 12-digit account ID, required/optional readiness status, one credential source, optional AssumeRole settings, and enabled collectors.

## Credential sources

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

- `default_chain` uses standard AWS SDK resolution: environment, shared files, container credentials, and instance metadata as applicable.
- `profile` uses AWS shared configuration, including `source_profile`, SSO, `credential_process`, and profile role chains.
- `static_env` stores only environment-variable names. The referenced variables must exist and be non-empty during `--check-config` and startup.

A source can back many AssumeRole targets, but at most one direct target. Direct targets use the source credentials as-is.

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

The role ARN must be exact, contain no wildcard, and match `account_id`. Each target gets an independent credentials cache. ExternalId values are read only from the named environment variable and never appear in configuration, logs, metrics, or debug output.

## Identity verification

Before a collector accesses billing APIs, the final credentials call STS `GetCallerIdentity`. The returned account must equal the configured `account_id`. Verification is target-scoped, single-flight, rate-limited, and cached after success. A mismatched Profile or environment source cannot publish data under the wrong target.

Authorization failure is a runtime target failure. It does not prevent unrelated targets from starting or refreshing.

## Multi-account patterns

- One payer credential source with exact AssumeRole targets is preferred for centralized operation.
- Independent Profiles are suitable for workstations and existing AWS shared configuration.
- Environment-backed static credentials are a last-resort integration; inject them through Secrets and rotate them externally.
- Organizations metadata never auto-discovers or creates targets.

Use the exact policies in [`examples/iam`](https://github.com/Sakuya1998/aws-cost-exporter/tree/master/examples/iam) and avoid wildcard `sts:AssumeRole` permissions.
