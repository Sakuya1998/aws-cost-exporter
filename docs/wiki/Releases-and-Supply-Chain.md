[English](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Releases-and-Supply-Chain) | [简体中文](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Releases-and-Supply-Chain-zh-CN)

# Releases and Supply Chain

The Wiki documents only the current stable release. Historical configuration contracts remain available through Git tags and GitHub Releases, not parallel Wiki trees.

## Release process

1. Merge a reviewed PR into `master` after CI passes.
2. Create an annotated SemVer tag such as `v1.0.0` from the verified `master` commit.
3. The tag-triggered Release workflow validates SemVer and installs pinned tools.
4. Trivy scans every image platform for HIGH and CRITICAL findings.
5. Buildx publishes the multi-architecture image with provenance and SBOM.
6. Cosign signs the image manifest digest with GitHub Actions keyless identity.
7. Helm packages and pushes the OCI chart, then cosign signs its digest.
8. GoReleaser builds archives, generates SPDX JSON SBOM files and checksums, and creates a Draft Release.
9. A maintainer independently verifies signatures and assets before publishing the Draft.

For v1 releases, the workflow also uploads a sanitized `release-audit-input`
artifact after GoReleaser completes. It contains the real tag, source SHA,
workflow URL, image and chart digests, 13 release asset names, cosign version,
certificate identity and issuer policy, and Trivy result. It is an input to
maintainer verification, not a substitute for independent `cosign verify`.

The repository-tracked [v1.0.0 checklist](https://github.com/Sakuya1998/aws-cost-exporter/blob/master/docs/releases/v1.0-checklist.md)
and [verification record](https://github.com/Sakuya1998/aws-cost-exporter/blob/master/docs/releases/v1.0.0-verification.md)
separate completed automated and supply-chain evidence from the 24-hour
stability, real AWS, and upgrade/rollback exercises explicitly deferred by the
maintainer. Deferred gates are not reported as passed.

The release workflow has `contents: write`, `packages: write`, and `id-token: write`; normal PR CI remains read-only.

## v1.0.0

- [GitHub Release](https://github.com/Sakuya1998/aws-cost-exporter/releases/tag/v1.0.0)
- [Verification record](https://github.com/Sakuya1998/aws-cost-exporter/blob/master/docs/releases/v1.0.0-verification.md)
- Image: `ghcr.io/sakuya1998/aws-cost-exporter:1.0.0`
- Chart: `ghcr.io/sakuya1998/charts/aws-cost-exporter:1.0.0`

The record fixes the source commit, CI runs, 13 release assets, VEX disposition,
attestations, certificate policy, and verified image/chart digests.

## Historical evidence

- [GitHub Release](https://github.com/Sakuya1998/aws-cost-exporter/releases/tag/v0.3.0)
- [Verification record](https://github.com/Sakuya1998/aws-cost-exporter/blob/master/docs/releases/v0.3.0-verification.md)
- Image: `ghcr.io/sakuya1998/aws-cost-exporter:0.3.0`
- Chart: `ghcr.io/sakuya1998/charts/aws-cost-exporter:0.3.0`

- [v0.1.5 verification](https://github.com/Sakuya1998/aws-cost-exporter/blob/master/docs/releases/v0.1.5-verification.md)
- [v0.1 release checklist](https://github.com/Sakuya1998/aws-cost-exporter/blob/master/docs/releases/v0.1-checklist.md)
- [All GitHub Releases](https://github.com/Sakuya1998/aws-cost-exporter/releases)

Historical records are immutable audit evidence. They are not current installation or configuration guidance.

## Consumer verification

Validate checksums for downloaded archives. For OCI artifacts, use cosign with the exact release workflow identity and GitHub Actions OIDC issuer from the release record. Pin the verified digest where tag mutability is unacceptable.
