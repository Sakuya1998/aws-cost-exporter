# Keep local development commands small and identical to their CI equivalents.
BINARY_PACKAGE := ./cmd/aws-cost-exporter
MODULE_PATH := github.com/sakuya1998/aws-cost-exporter
VERSION ?= dev
REVISION ?= unknown
BUILD_DATE ?= unknown
LDFLAGS := -X $(MODULE_PATH)/internal/version.version=$(VERSION)
LDFLAGS += -X $(MODULE_PATH)/internal/version.revision=$(REVISION)
LDFLAGS += -X $(MODULE_PATH)/internal/version.buildDate=$(BUILD_DATE)

.PHONY: build test lint clean

# build compiles the exporter executable for the current platform.
build:
	go build -trimpath -ldflags "$(LDFLAGS)" $(BINARY_PACKAGE)

# test runs every unit test in the module.
test:
	go test ./...

# test-race runs the module tests with the race detector enabled.
test-race:
	go test -race ./...

# lint applies the repository's static-analysis baseline.
lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2 run ./...

# clean removes executable artifacts produced by the build target.
clean:
	go clean
