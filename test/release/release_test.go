package release_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestGoReleaserBuildsVersionedMultiPlatformArtifacts(t *testing.T) {
	content := read(t, filepath.Join("..", "..", ".goreleaser.yaml"))
	var document any
	if err := yaml.Unmarshal([]byte(content), &document); err != nil {
		t.Fatalf("parse GoReleaser config: %v", err)
	}
	for _, fragment := range []string{
		"version: 2", "main: ./cmd/aws-cost-exporter", "CGO_ENABLED=0",
		"goos: [linux, darwin, windows]", "goarch: [amd64, arm64]",
		"internal/version.version={{.Version}}",
		"internal/version.revision={{.FullCommit}}",
		"internal/version.buildDate={{.Date}}",
		"formats: [tar.gz]", "goos: windows", "formats: [zip]",
		"name_template: checksums.txt", "sboms:", "artifacts: archive",
		"prerelease: auto", "draft: true", "use: git",
	} {
		if !strings.Contains(content, fragment) {
			t.Errorf("GoReleaser config lacks %q", fragment)
		}
	}
}

func TestReleaseWorkflowHasMinimalSignedPublishingContract(t *testing.T) {
	content := read(t, filepath.Join("..", "..", ".github", "workflows", "release.yml"))
	var document any
	if err := yaml.Unmarshal([]byte(content), &document); err != nil {
		t.Fatalf("parse release workflow: %v", err)
	}
	root, ok := document.(map[string]any)
	if !ok {
		t.Fatal("release workflow root is not a mapping")
	}
	permissions, ok := root["permissions"].(map[string]any)
	if !ok {
		t.Fatal("release workflow permissions are not a mapping")
	}
	wantPermissions := map[string]string{
		"contents": "write", "packages": "write", "id-token": "write",
	}
	if len(permissions) != len(wantPermissions) {
		t.Errorf("release permissions = %v, want exactly %v", permissions, wantPermissions)
	}
	for name, want := range wantPermissions {
		if permissions[name] != want {
			t.Errorf("release permission %s = %v, want %s", name, permissions[name], want)
		}
	}
	for _, fragment := range []string{
		"tags: [\"v*\"]", "contents: write", "packages: write", "id-token: write",
		"go-version: 1.26.5",
		`^v[0-9]+\.[0-9]+\.[0-9]+`, "goreleaser release --clean",
		`IMAGE=ghcr.io/${GITHUB_REPOSITORY,,}`,
		`CHART_REPOSITORY=ghcr.io/${GITHUB_REPOSITORY_OWNER,,}/charts`,
		`CHART=ghcr.io/${GITHUB_REPOSITORY_OWNER,,}/charts/aws-cost-exporter`,
		"linux/amd64,linux/arm64",
		"--provenance=mode=max", "--sbom=true", "containerimage.digest",
		`metadata_file="$RUNNER_TEMP/image-metadata.json"`, `--metadata-file "$metadata_file"`,
		"trivy image", "cosign sign --yes", "helm package", "helm push",
		"charts/aws-cost-exporter", "GITHUB_REF_NAME", "!= *-*",
		`'.["containerimage.digest"]'`, `--app-version "$VERSION"`, `"oci://$CHART_REPOSITORY"`,
		`cosign sign --yes "$CHART@$digest"`, `--platform "$platform"`,
	} {
		if !strings.Contains(content, fragment) {
			t.Errorf("release workflow lacks %q", fragment)
		}
	}
	for _, forbidden := range []string{
		"security-events: write", "actions: write", "--metadata-file image-metadata.json",
	} {
		if strings.Contains(content, forbidden) {
			t.Errorf("release workflow grants unnecessary permission %q", forbidden)
		}
	}
	uses := regexp.MustCompile(`uses:\s*[\w./-]+@([^\s#]+)`).FindAllStringSubmatch(content, -1)
	if len(uses) == 0 {
		t.Fatal("release workflow has no actions")
	}
	sha := regexp.MustCompile(`^[0-9a-f]{40}$`)
	for _, match := range uses {
		if !sha.MatchString(match[1]) {
			t.Errorf("release action is not SHA pinned: %s", match[0])
		}
	}
}

func TestV01ChecklistCoversReleaseGates(t *testing.T) {
	content := read(t, filepath.Join("..", "..", "docs", "releases", "v0.1-checklist.md"))
	for _, fragment := range []string{
		"# v0.1.0 release checklist", "./test/perf/...", "./test/release/...",
		"race", "does not call AWS", "8 `GetCostAndUsage` + 1 `GetCostForecast`",
		"perf API-budget gate (8+1) does not apply", "pagination_pages_total",
		"series_limit=1000", "15s", "cosign verify", "Trivy", "ROADMAP.md", "arm64",
	} {
		if !strings.Contains(content, fragment) {
			t.Errorf("v0.1 checklist lacks %q", fragment)
		}
	}
}

func TestV01ChecklistReferencesAutomatedSuites(t *testing.T) {
	for _, path := range []string{"test/perf/gate_test.go", "test/e2e/e2e_test.go", "test/release/release_test.go"} {
		if _, err := os.Stat(filepath.Join("..", "..", filepath.FromSlash(path))); err != nil {
			t.Fatalf("checklist dependency missing %s: %v", path, err)
		}
	}
}

func TestV014VerificationRecordPinsArtifactsAndIdentity(t *testing.T) {
	content := read(t, filepath.Join("..", "..", "docs", "releases", "v0.1.4-verification.md"))
	for _, fragment := range []string{
		"cosign v3.1.1",
		"release.yml@refs/tags/v0.1.4",
		"https://token.actions.githubusercontent.com",
		"sha256:84c9004e6d8f0aaefa8de3e64171b623ff89b3ff70006cdda470d77a5d335e60",
		"sha256:664425f6a5eeda58870d25db83c08b0af351c8ef0b825b412ca2ecee74f9d8e0",
		"transparency log",
	} {
		if !strings.Contains(content, fragment) {
			t.Errorf("v0.1.4 verification record lacks %q", fragment)
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
