# Architecture

The v1.0 stabilization contract preserves the v0.3 modular-monolith design:
background collection, typed domain results, copy-on-write partial publication,
memory-only snapshots, and AWS-free Prometheus scrapes. It deliberately keeps
one replica and no leader election or shared cache.

The maintained architecture guide for the current stable release lives in the repository-backed Wiki source:

- [Architecture source](docs/wiki/Architecture.md)
- [Published English Wiki](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Architecture)
- [Published 简体中文 Wiki](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Architecture-zh-CN)

Architecture decisions remain authoritative in [docs/adr](docs/adr). The central invariant is unchanged: background collectors publish immutable snapshots, and Prometheus scrapes never initiate AWS or Athena requests.

The reference capacity is 20 targets and 20,000+ business series on 2 vCPU and
512 MiB. See [the v1 capacity and stability SLO](docs/operations/v1-slo.md).
