package ci_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestCIWorkflowEnforcesQualityAndAssetChecks(t *testing.T) {
	content := read(t, filepath.Join("..", "..", ".github", "workflows", "ci.yml"))
	var document any
	if err := yaml.Unmarshal([]byte(content), &document); err != nil {
		t.Fatalf("parse CI workflow: %v", err)
	}
	for _, fragment := range []string{
		"pull_request:", "branches: [master]", "contents: read", "go-version: [\"1.24.x\", stable]",
		"gofmt -l", "goimports", "go vet ./...", "golangci-lint-action",
		"version: v2.12.2",
		"govulncheck", "gosec", "go test -race", "coverage < 75",
		"./test/integration/...", "./test/e2e/...", "./test/perf/...",
		"./test/chart/...", "./test/dashboard/...", "./test/rules/...",
		"./test/docs/...", "./test/release/...",
		"prometheus_version=2.55.1", "sha256sum --check --strict", "promtool",
		"kubeconform", "version: v3.21.3", "./test/container/...",
	} {
		if !strings.Contains(content, fragment) {
			t.Errorf("CI workflow lacks %q", fragment)
		}
	}
	for _, forbidden := range []string{
		"contents: write", "packages: write", "id-token: write",
		"go install github.com/prometheus/prometheus/cmd/promtool@",
	} {
		if strings.Contains(content, forbidden) {
			t.Errorf("PR workflow grants forbidden permission %q", forbidden)
		}
	}
	uses := regexp.MustCompile(`uses:\s*[\w./-]+@([^\s#]+)`).FindAllStringSubmatch(content, -1)
	if len(uses) == 0 {
		t.Fatal("CI workflow has no actions")
	}
	sha := regexp.MustCompile(`^[0-9a-f]{40}$`)
	for _, match := range uses {
		if !sha.MatchString(match[1]) {
			t.Errorf("action is not SHA pinned: %s", match[0])
		}
	}
}

func TestDependabotAndGolangCIBaselines(t *testing.T) {
	dependabot := read(t, filepath.Join("..", "..", ".github", "dependabot.yml"))
	var document any
	if err := yaml.Unmarshal([]byte(dependabot), &document); err != nil {
		t.Fatalf("parse Dependabot config: %v", err)
	}
	for _, ecosystem := range []string{"gomod", "github-actions", "docker"} {
		if !strings.Contains(dependabot, `package-ecosystem: "`+ecosystem+`"`) {
			t.Errorf("Dependabot lacks %s updates", ecosystem)
		}
	}
	if strings.Count(dependabot, "groups:") < 3 {
		t.Error("Dependabot updates are not grouped by ecosystem")
	}
	lint := read(t, filepath.Join("..", "..", ".golangci.yml"))
	if err := yaml.Unmarshal([]byte(lint), &document); err != nil {
		t.Fatalf("parse golangci config: %v", err)
	}
	for _, name := range []string{"errcheck", "govet", "ineffassign", "staticcheck", "unused"} {
		if !strings.Contains(lint, "- "+name) {
			t.Errorf("golangci baseline lacks %s", name)
		}
	}
	if !strings.Contains(lint, `version: "2"`) || !strings.Contains(lint, "default: none") {
		t.Error("golangci baseline is not a v2 explicit linter set")
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}
