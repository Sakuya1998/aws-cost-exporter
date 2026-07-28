# Architecture

The maintained architecture guide for the current stable release lives in the repository-backed Wiki source:

- [Architecture source](docs/wiki/Architecture.md)
- [Published English Wiki](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Architecture)
- [Published 简体中文 Wiki](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Architecture-zh-CN)

Architecture decisions remain authoritative in [docs/adr](docs/adr). The central invariant is unchanged: background collectors publish immutable snapshots, and Prometheus scrapes never initiate AWS or Athena requests.
