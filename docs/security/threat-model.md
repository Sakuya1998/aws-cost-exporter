# AWS Cost Exporter v1 Threat Model

## Scope

This model covers the v1 single-process exporter, its configuration and
credentials, AWS API and Athena traffic, Prometheus exposition, container and
Helm deployment, CI, and release artifacts. It assumes an operator controls the
deployment environment and explicitly grants read access to billing systems.

## Assets

- AWS credentials, AssumeRole sessions, ExternalId values, and credential
  source files.
- Account identifiers, organization metadata, budget names, tag values, cost
  amounts, forecasts, commitment summaries, and anomaly summaries.
- Exporter configuration, immutable in-memory snapshots, logs, and `/metrics`
  financial telemetry.
- Source code, CI credentials, release artifacts, SBOMs, provenance, signatures,
  and the GitHub release identity.

## Trust boundaries

1. Operators and the deployment platform supply YAML, environment variables,
   mounted profiles, Secrets, network policy, and resource limits.
2. The AWS SDK crosses the network boundary to STS, Cost Explorer,
   Organizations, Budgets, and Athena; all responses are untrusted input.
3. CUR data crosses S3 and Glue boundaries before Athena returns typed rows.
4. Prometheus and any user of `/metrics` cross a financial-data disclosure
   boundary. The exporter does not provide built-in TLS or authentication.
5. GitHub Actions, registries, Sigstore, and release consumers cross the
   software supply-chain boundary.

Endpoint overrides are intended for local tests. A malicious endpoint override
can receive signed requests and attempt credential misuse. Protect endpoint
override settings because configuration write access is therefore equivalent
to authority to use the configured AWS
credentials. Production configuration must be protected accordingly.

## Threats

### Spoofing

An attacker may supply a credential source for the wrong account, impersonate
an AWS endpoint through configuration, or substitute an unsigned image or
chart. Target identity verification, TLS endpoint defaults, explicit account
IDs, digest pinning, provenance, and cosign verification reduce these risks.

### Tampering

An attacker with configuration, Secret, image, or workflow write access could
redirect signed requests, broaden IAM access, alter queries, or modify release
outputs. Exact configuration parsing, fixed generated SQL, protected branches,
SHA-pinned Actions, read-only containers, and reviewed release evidence protect
these boundaries.

### Repudiation

An unrecorded release or configuration change could make it difficult to prove
which code generated an artifact. Immutable source commits, checksums, SBOMs,
provenance, keyless signature identities, workflow URLs, and release audit
records provide attribution. They do not prove that an operator used a
particular runtime configuration.

### Information disclosure

`/metrics` contains sensitive financial telemetry even though credentials,
account email, raw AWS responses, and ExternalId values are excluded. Logs and
errors can also disclose identifiers if raw upstream text is emitted.
Production deployments must keep the service private and restrict Prometheus,
debug, logs, Secrets, and snapshot access to authorized operators.

### Denial of service

Unbounded targets, pages, tag values, series, retries, concurrency, or refresh
frequency could exhaust CPU/memory or create material AWS and Athena request
cost. Strict configuration limits, allowlists, overflow aggregation, bounded
pagination, global and target rate limits, backoff, single-flight collection,
and the single-replica Helm contract limit this exposure.

### Elevation of privilege

Wildcard `sts:AssumeRole`, writable credential files, a privileged pod, or an
over-broad Athena/S3 policy could turn read-only billing access into broader AWS
authority. Capability-specific IAM policies, exact role ARNs, ExternalId,
non-root execution, dropped capabilities, read-only filesystems, and disabled
ServiceAccount token mounting reduce the impact.

## Mitigations

- Use one narrowly scoped credential source per trust boundary and verify the
  final STS account against the configured target.
- Keep endpoint overrides out of production and protect configuration with the
  same controls as credentials.
- Expose HTTP only on private networks or behind authenticated TLS proxies.
- Enable only required collectors and use the documented request, pagination,
  series, tag, concurrency, and rate limits.
- Keep `replicaCount: 1`; overlapping replicas duplicate paid AWS requests.
- Run secret, vulnerability, static-analysis, container, Helm, contract, and
  supply-chain checks before release.
- Verify image and chart digests, SBOMs, provenance, and cosign identities.

## Residual risks

AWS billing data can be delayed or corrected, IAM actions without resource-level
permissions still require `Resource: *`, and authorized metrics readers can see
financial telemetry. A compromised deployment administrator can use configured
credentials or replace configuration. In-memory snapshots are lost on restart,
and a single replica creates a temporary metrics outage during upgrades.

## Non-goals

The exporter is not a billing reconciliation system, secrets manager, identity
provider, TLS terminator, multi-tenant authorization layer, persistent cache,
or high-availability coordinator. It does not create CUR, S3, Glue, Athena,
Budgets, anomaly subscriptions, or AWS accounts.
