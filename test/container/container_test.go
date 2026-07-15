package container_test

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestContainerAssetsEnforceSecurityContract(t *testing.T) {
	root := filepath.Join("..", "..")
	dockerfile := read(t, filepath.Join(root, "Dockerfile"))
	for _, required := range []string{
		"FROM --platform=$BUILDPLATFORM golang:1.26.5-alpine3.24 AS builder",
		"ARG TARGETOS", "ARG TARGETARCH", "CGO_ENABLED=0",
		"GOOS=$TARGETOS", "GOARCH=$TARGETARCH", "-trimpath",
		"gcr.io/distroless/static-debian12:nonroot",
		"USER 65532:65532", "EXPOSE 8080",
		`ENTRYPOINT ["/aws-cost-exporter"]`,
		"org.opencontainers.image.licenses=\"Apache-2.0\"",
	} {
		if !strings.Contains(dockerfile, required) {
			t.Errorf("Dockerfile missing %q", required)
		}
	}
	compose := read(t, filepath.Join(root, "docker-compose.yml"))
	for _, required := range []string{
		"read_only: true", `user: "${AWS_HOST_UID:-65532}:${AWS_HOST_GID:-65532}"`, "no-new-privileges:true",
		"cap_drop:", "- ALL", "aws-cost-exporter.example.yaml:/etc/aws-cost-exporter/config.yaml:ro",
		"${HOME}/.aws:/home/nonroot/.aws:ro",
	} {
		if !strings.Contains(compose, required) {
			t.Errorf("docker-compose.yml missing %q", required)
		}
	}
	for _, forbidden := range []string{"AWS_ACCESS_KEY_ID:", "AWS_SECRET_ACCESS_KEY:", "AWS_SESSION_TOKEN:"} {
		if strings.Contains(compose, forbidden) {
			t.Errorf("docker-compose.yml must not declare %s", forbidden)
		}
	}
	ignore := read(t, filepath.Join(root, ".dockerignore"))
	for _, required := range []string{".git", "test", "aws-cost-exporter.exe"} {
		if !strings.Contains(ignore, required) {
			t.Errorf(".dockerignore missing %q", required)
		}
	}
}

func TestMultiArchitectureBuild(t *testing.T) {
	docker, err := exec.LookPath("docker")
	if err != nil {
		t.Skip("Docker is not installed")
	}
	if err := exec.Command(docker, "buildx", "version").Run(); err != nil {
		t.Skip("Docker buildx is not installed")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	archive := filepath.Join(t.TempDir(), "image.tar")
	builder := fmt.Sprintf("aws-cost-exporter-test-%d", time.Now().UnixNano())
	run(t, root, docker, "buildx", "create", "--name", builder, "--driver", "docker-container")
	t.Cleanup(func() { _ = exec.Command(docker, "buildx", "rm", "-f", builder).Run() })
	run(t, root, docker, "buildx", "inspect", builder, "--bootstrap")
	run(t, root, docker, "buildx", "build", "--builder", builder, "--platform", "linux/amd64,linux/arm64",
		"--output", "type=oci,dest="+archive, ".")
}

func TestContainerSmoke(t *testing.T) {
	docker, err := exec.LookPath("docker")
	if err != nil {
		t.Skip("Docker is not installed")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	image := fmt.Sprintf("aws-cost-exporter:test-%d", time.Now().UnixNano())
	run(t, root, docker, "build", "--build-arg", "VERSION=e2e", "--build-arg",
		"REVISION=test", "--build-arg", "BUILD_DATE=2026-07-13T00:00:00Z", "-t", image, ".")
	t.Cleanup(func() { _ = exec.Command(docker, "image", "rm", "-f", image).Run() })
	configPath := filepath.Join(root, "configs", "aws-cost-exporter.example.yaml")
	container := strings.TrimSpace(run(t, root, docker, "run", "-d", "--read-only",
		"--user", "65532:65532", "--security-opt", "no-new-privileges:true",
		"--cap-drop", "ALL", "-e", "AWS_EC2_METADATA_DISABLED=true",
		"--mount", "type=bind,src="+configPath+",dst=/etc/aws-cost-exporter/config.yaml,readonly",
		"-p", "127.0.0.1::8080", image))
	t.Cleanup(func() { _ = exec.Command(docker, "rm", "-f", container).Run() })
	if user := strings.TrimSpace(run(t, root, docker, "inspect", "--format", "{{.Config.User}}", container)); user != "65532:65532" {
		t.Fatalf("container user = %q", user)
	}
	port := strings.TrimSpace(run(t, root, docker, "port", container, "8080/tcp"))
	awaitHealth(t, "http://"+port+"/healthz")
	run(t, root, docker, "rm", "-f", container)
	if err := exec.Command(docker, "run", "--rm", "--entrypoint", "/bin/sh", image).Run(); err == nil {
		t.Fatal("distroless image unexpectedly contains /bin/sh")
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func run(t *testing.T, directory, command string, arguments ...string) string {
	t.Helper()
	process := exec.Command(command, arguments...)
	process.Dir = directory
	output, err := process.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", command, arguments, err, output)
	}
	return string(output)
}

func awaitHealth(t *testing.T, url string) {
	t.Helper()
	client := &http.Client{Timeout: 500 * time.Millisecond}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		response, err := client.Get(url)
		if err == nil {
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("%s did not become healthy", url)
}
