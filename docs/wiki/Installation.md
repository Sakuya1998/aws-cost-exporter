[English](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Installation) | [简体中文](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Installation-zh-CN)

# Installation

## Release artifacts

Download v0.3.0 archives and checksums from the [GitHub Release](https://github.com/Sakuya1998/aws-cost-exporter/releases/tag/v0.3.0). Releases include Linux, Windows, and macOS archives for amd64 and arm64, SPDX JSON SBOM files, and `checksums.txt`.

## Build from source

```bash
git clone https://github.com/Sakuya1998/aws-cost-exporter.git
cd aws-cost-exporter
git checkout v0.3.0
make build
./aws-cost-exporter --version
```

## Docker

```bash
docker pull ghcr.io/sakuya1998/aws-cost-exporter:0.3.0
docker compose up --build
```

Mount AWS shared configuration read-only or inject only environment-variable references required by configured credential sources. Do not bake credentials into an image.

## Helm

```bash
helm install aws-cost-exporter \
  oci://ghcr.io/sakuya1998/charts/aws-cost-exporter \
  --version 0.3.0 \
  --set config.data.targets[0].account_id=444455556666
```

Use `config.secretEnvRefs` for existing Secrets and `awsSharedConfig.existingSecret` for mounted AWS Profile files. The chart deliberately defaults to `replicaCount: 1`.

## Verify OCI signatures

The image and Helm chart are signed by the tag-triggered release workflow. Use the exact identity and issuer from the [v0.3.0 verification record](https://github.com/Sakuya1998/aws-cost-exporter/blob/master/docs/releases/v0.3.0-verification.md). Pin verified digests when immutability is required.
