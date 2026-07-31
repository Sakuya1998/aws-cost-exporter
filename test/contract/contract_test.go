package contract_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const contractVersion = "v1.0.0"

func TestV1ContractFixturesAreParseableAndReferenced(t *testing.T) {
	root := repositoryRoot(t)
	contracts := []struct {
		name        string
		fixture     string
		packageTest string
	}{
		{
			name:        "config",
			fixture:     filepath.Join("internal", "config", "testdata", "v1", "config-contract.json"),
			packageTest: filepath.Join("internal", "config", "contract_test.go"),
		},
		{
			name:        "metrics",
			fixture:     filepath.Join("internal", "metrics", "testdata", "v1", "metrics-contract.json"),
			packageTest: filepath.Join("internal", "metrics", "contract_test.go"),
		},
		{
			name:        "http",
			fixture:     filepath.Join("internal", "httpserver", "testdata", "v1", "http-contract.json"),
			packageTest: filepath.Join("internal", "httpserver", "contract_test.go"),
		},
	}

	for _, contract := range contracts {
		t.Run(contract.name, func(t *testing.T) {
			fixture := read(t, filepath.Join(root, contract.fixture))
			var header struct {
				Version string `json:"version"`
			}
			if err := json.Unmarshal(fixture, &header); err != nil {
				t.Fatalf("parse %s: %v", contract.fixture, err)
			}
			if header.Version != contractVersion {
				t.Errorf("%s version = %q, want %q", contract.fixture, header.Version, contractVersion)
			}

			packageTest := read(t, filepath.Join(root, contract.packageTest))
			if !strings.Contains(string(packageTest), filepath.Base(contract.fixture)) {
				t.Errorf("%s does not reference %s", contract.packageTest, contract.fixture)
			}
		})
	}
}

func TestCIEnforcesContractsWithoutUpdatingBaselines(t *testing.T) {
	workflow := string(read(t, filepath.Join(repositoryRoot(t), ".github", "workflows", "ci.yml")))
	if !strings.Contains(workflow, "./test/contract/...") {
		t.Error("CI workflow does not run ./test/contract/...")
	}
	if strings.Contains(workflow, "-update-contract") {
		t.Error("CI workflow must not update reviewed contract baselines")
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	return root
}

func read(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return content
}
