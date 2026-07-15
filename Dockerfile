# syntax=docker/dockerfile:1.7

FROM --platform=$BUILDPLATFORM golang:1.26.5-alpine3.24 AS builder

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG REVISION=unknown
ARG BUILD_DATE=unknown

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build \
    -buildvcs=false -trimpath \
    -ldflags="-s -w \
    -X github.com/sakuya1998/aws-cost-exporter/internal/version.version=${VERSION} \
    -X github.com/sakuya1998/aws-cost-exporter/internal/version.revision=${REVISION} \
    -X github.com/sakuya1998/aws-cost-exporter/internal/version.buildDate=${BUILD_DATE}" \
    -o /out/aws-cost-exporter ./cmd/aws-cost-exporter

FROM gcr.io/distroless/static-debian12:nonroot

ARG VERSION=dev
ARG REVISION=unknown
ARG BUILD_DATE=unknown
LABEL org.opencontainers.image.title="aws-cost-exporter" \
      org.opencontainers.image.description="Prometheus exporter for AWS cost data" \
      org.opencontainers.image.source="https://github.com/sakuya1998/aws-cost-exporter" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${REVISION}" \
      org.opencontainers.image.created="${BUILD_DATE}"

COPY --from=builder --chown=65532:65532 /out/aws-cost-exporter /aws-cost-exporter

USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/aws-cost-exporter"]
CMD ["--config", "/etc/aws-cost-exporter/config.yaml"]
