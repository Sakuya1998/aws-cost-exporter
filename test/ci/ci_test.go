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
	root, ok := document.(map[string]any)
	if !ok {
		t.Fatalf("CI workflow root has type %T", document)
	}
	jobs, ok := root["jobs"].(map[string]any)
	if !ok {
		t.Fatalf("CI workflow jobs has type %T", root["jobs"])
	}
	assetsYAML, err := yaml.Marshal(jobs["assets"])
	if err != nil {
		t.Fatalf("marshal CI assets job: %v", err)
	}
	if !strings.Contains(string(assetsYAML), "./test/contract/...") {
		t.Error("CI assets job does not run ./test/contract/...")
	}
	for _, fragment := range []string{
		"pull_request:", "branches: [master]", "contents: read", "go-version: [\"1.24.x\", stable]",
		"gofmt -l", "goimports", "go vet ./...", "golangci-lint-action",
		"version: v2.12.2",
		"govulncheck", "gosec", "go test -race", "coverage < 79", "core coverage < 85%",
		"core-coverage.out", "./internal/domain/...", "./internal/cache/...", "./internal/scheduler/...",
		"./internal/aws/common/...", "./internal/aws/clientfactory/...",
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
		"-update-contract",
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

func TestMakefileExposesReviewedContractCheck(t *testing.T) {
	content := read(t, filepath.Join("..", "..", "Makefile"))
	phony := regexp.MustCompile(`(?m)^\.PHONY:.*\bcontract\b`)
	if !phony.MatchString(content) {
		t.Error("Makefile .PHONY does not include contract")
	}
	contract := regexp.MustCompile(`(?m)^contract:\r?\n\tgo test -count=1 ./internal/config ./internal/metrics ./internal/httpserver ./test/contract/\.\.\.$`)
	if !contract.MatchString(content) {
		t.Error("Makefile contract target does not run the reviewed contract suites")
	}
}

func TestDependabotAndGolangCIBaselines(t *testing.T) {
	dependabot := read(t, filepath.Join("..", "..", ".github", "dependabot.yml"))
	var document any
	if err := yaml.Unmarshal([]byte(dependabot), &document); err != nil {
		t.Fatalf("parse Dependabot config: %v", err)
	}
	for _, ecosystem := range []string{"gomod", "github-actions", "docker", "pip"} {
		if !strings.Contains(dependabot, `package-ecosystem: "`+ecosystem+`"`) {
			t.Errorf("Dependabot lacks %s updates", ecosystem)
		}
	}
	if strings.Count(dependabot, "groups:") < 4 {
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

func TestWikiWorkflowIsSafeAndScoped(t *testing.T) {
	content := read(t, filepath.Join("..", "..", ".github", "workflows", "wiki.yml"))
	var document any
	if err := yaml.Unmarshal([]byte(content), &document); err != nil {
		t.Fatalf("parse wiki workflow: %v", err)
	}
	for _, fragment := range []string{
		"branches: [master]", "workflow_dispatch:", "docs/wiki/**", "contents: write",
		"cancel-in-progress: false", "${GITHUB_REPOSITORY}.wiki.git", "docs/wiki",
		"README.md", "github-actions[bot]", "SOURCE_SHA", `git -C "$WIKI_DIR" push`,
	} {
		if !strings.Contains(content, fragment) {
			t.Errorf("wiki workflow lacks %q", fragment)
		}
	}
	for _, forbidden := range []string{
		"pull_request_target", "packages: write", "id-token: write", "secrets.WIKI_TOKEN",
	} {
		if strings.Contains(content, forbidden) {
			t.Errorf("wiki workflow contains forbidden fragment %q", forbidden)
		}
	}
	uses := regexp.MustCompile(`uses:\s*[\w./-]+@([^\s#]+)`).FindAllStringSubmatch(content, -1)
	if len(uses) == 0 {
		t.Fatal("wiki workflow has no actions")
	}
	sha := regexp.MustCompile(`^[0-9a-f]{40}$`)
	for _, match := range uses {
		if !sha.MatchString(match[1]) {
			t.Errorf("wiki action is not SHA pinned: %s", match[0])
		}
	}
}

func TestPagesWorkflowIsSafePinnedAndScoped(t *testing.T) {
	content := read(t, filepath.Join("..", "..", ".github", "workflows", "pages.yml"))
	var document any
	if err := yaml.Unmarshal([]byte(content), &document); err != nil {
		t.Fatalf("parse Pages workflow: %v", err)
	}
	root, ok := document.(map[string]any)
	if !ok {
		t.Fatalf("Pages workflow root has type %T", document)
	}
	permissions, ok := root["permissions"].(map[string]any)
	if !ok || len(permissions) != 1 || permissions["contents"] != "read" {
		t.Errorf("workflow permissions=%v, want only contents: read", root["permissions"])
	}
	jobs, ok := root["jobs"].(map[string]any)
	if !ok {
		t.Fatalf("Pages workflow jobs has type %T", root["jobs"])
	}
	deploy, ok := jobs["deploy"].(map[string]any)
	if !ok {
		t.Fatalf("Pages deploy job has type %T", jobs["deploy"])
	}
	deployPermissions, ok := deploy["permissions"].(map[string]any)
	if !ok || len(deployPermissions) != 2 || deployPermissions["pages"] != "write" || deployPermissions["id-token"] != "write" {
		t.Errorf("deploy permissions=%v, want pages: write and id-token: write", deploy["permissions"])
	}
	buildYAML, err := yaml.Marshal(jobs["build"])
	if err != nil {
		t.Fatalf("marshal Pages build job: %v", err)
	}
	deployYAML, err := yaml.Marshal(deploy)
	if err != nil {
		t.Fatalf("marshal Pages deploy job: %v", err)
	}
	if strings.Contains(string(buildYAML), "actions/configure-pages") || !strings.Contains(string(deployYAML), "actions/configure-pages") {
		t.Error("configure-pages must run in the deploy job that has Pages permission")
	}
	for _, fragment := range []string{
		"branches: [master]", "workflow_dispatch:", "docs/wiki/**", "mkdocs.yml", "requirements-docs.txt",
		"contents: read", "pages: write", "id-token: write", "cancel-in-progress: false",
		"go run ./tools/pages", "mkdocs build --strict", "actions/configure-pages", "actions/upload-pages-artifact",
		"actions/deploy-pages", "github-pages", "url: ${{ steps.deployment.outputs.page_url }}",
	} {
		if !strings.Contains(content, fragment) {
			t.Errorf("Pages workflow lacks %q", fragment)
		}
	}
	for _, forbidden := range []string{
		"pull_request_target", "contents: write", "packages: write", "secrets.",
		"11d5960a326750d5838078e36cf38b85af677262", "ece7cb06caefa5fff74198d8649806c4678c61a1",
		"983d7736d9b0ae728b81ab479565c72886d7745b", "56afc609e74202658d3ffba0e8f6dda462b719fa",
		"d6db90164ac5ed86f2b6aed7e0febac5b3c0c03e",
	} {
		if strings.Contains(content, forbidden) {
			t.Errorf("Pages workflow contains forbidden fragment %q", forbidden)
		}
	}
	uses := regexp.MustCompile(`uses:\s*[\w./-]+@([^\s#]+)`).FindAllStringSubmatch(content, -1)
	if len(uses) < 5 {
		t.Fatalf("Pages workflow has %d pinned actions, want at least 5", len(uses))
	}
	sha := regexp.MustCompile(`^[0-9a-f]{40}$`)
	for _, match := range uses {
		if !sha.MatchString(match[1]) {
			t.Errorf("Pages action is not SHA pinned: %s", match[0])
		}
	}
}

func TestPagesMkDocsConfigurationUsesGeneratedWikiSource(t *testing.T) {
	content := read(t, filepath.Join("..", "..", "mkdocs.yml"))
	var document any
	if err := yaml.Unmarshal([]byte(content), &document); err != nil {
		t.Fatalf("parse MkDocs config: %v", err)
	}
	for _, fragment := range []string{
		"site_url: https://sakuya1998.github.io/aws-cost-exporter/", "docs_dir: .pages-docs", "name: material",
		"search", "content.code.copy", "pymdownx.superfences", "Home-zh-CN.md",
		"Configuration-Reference.md", "Configuration-Reference-zh-CN.md",
	} {
		if !strings.Contains(content, fragment) {
			t.Errorf("MkDocs config lacks %q", fragment)
		}
	}
	for _, brokenEditLinkSetting := range []string{"content.action.edit", "edit_uri:"} {
		if strings.Contains(content, brokenEditLinkSetting) {
			t.Errorf("MkDocs config enables generated-source edit links through %q", brokenEditLinkSetting)
		}
	}

	requirements := read(t, filepath.Join("..", "..", "requirements-docs.txt"))
	for _, dependency := range []string{"mkdocs==1.6.1", "mkdocs-material==9.7.7"} {
		if !strings.Contains(requirements, dependency) {
			t.Errorf("documentation requirements lack %q", dependency)
		}
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
