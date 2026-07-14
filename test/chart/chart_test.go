package chart_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestChartAssetsContainRuntimeContracts(t *testing.T) {
	chart := chartPath(t)
	for _, name := range []string{
		"Chart.yaml", "values.yaml", "templates/_helpers.tpl",
		"templates/deployment.yaml", "templates/service.yaml",
		"templates/configmap.yaml", "templates/serviceaccount.yaml",
	} {
		if _, err := os.Stat(filepath.Join(chart, name)); err != nil {
			t.Errorf("required chart file %s: %v", name, err)
		}
	}
	for _, name := range []string{"Chart.yaml", "values.yaml"} {
		content := read(t, filepath.Join(chart, name))
		var document map[string]any
		if err := yaml.Unmarshal([]byte(content), &document); err != nil {
			t.Errorf("%s is invalid YAML: %v", name, err)
		}
	}
	values := read(t, filepath.Join(chart, "values.yaml"))
	for _, required := range []string{
		"repository: ghcr.io/sakuya1998/aws-cost-exporter",
		"max_pages: 50",
		"runAsUser: 65532", "runAsGroup: 65532", "runAsNonRoot: true",
		"readOnlyRootFilesystem: true", "allowPrivilegeEscalation: false",
		"path: /healthz", "path: /ready",
		"serviceMonitor:", "prometheusRule:", "networkPolicy:", "podDisruptionBudget:",
	} {
		if !strings.Contains(values, required) {
			t.Errorf("values.yaml missing %q", required)
		}
	}
	chartYAML := read(t, filepath.Join(chart, "Chart.yaml"))
	for _, required := range []string{
		"home: https://github.com/sakuya1998/aws-cost-exporter",
		"https://github.com/sakuya1998/aws-cost-exporter",
	} {
		if !strings.Contains(chartYAML, required) {
			t.Errorf("Chart.yaml missing %q", required)
		}
	}
	deployment := read(t, filepath.Join(chart, "templates", "deployment.yaml"))
	for _, required := range []string{
		"checksum/config:", "livenessProbe:", "readinessProbe:",
		"securityContext:", "config.yaml", "readOnly: true",
	} {
		if !strings.Contains(deployment, required) {
			t.Errorf("deployment.yaml missing %q", required)
		}
	}
}

func TestHelmLintAndDefaultTemplate(t *testing.T) {
	helm, err := exec.LookPath("helm")
	if err != nil {
		t.Fatal("Helm 3 is required to validate the chart")
	}
	chart := chartPath(t)
	run(t, helm, "lint", chart)
	rendered := run(t, helm, "template", "e2e", chart)
	for _, required := range []string{
		"kind: Deployment", "kind: Service", "kind: ConfigMap",
		"kind: ServiceAccount", "checksum/config:", "runAsNonRoot: true",
		"path: /healthz", "path: /ready",
	} {
		if !strings.Contains(rendered, required) {
			t.Errorf("rendered chart missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"kind: ServiceMonitor", "kind: PrometheusRule",
		"kind: NetworkPolicy", "kind: PodDisruptionBudget",
	} {
		if strings.Contains(rendered, forbidden) {
			t.Errorf("default chart unexpectedly rendered %q", forbidden)
		}
	}
}

func TestFullTemplatePassesKubeconform(t *testing.T) {
	helm := requireTool(t, "helm")
	kubeconform := requireTool(t, "kubeconform")
	chart := chartPath(t)
	values := filepath.Join(chart, "..", "..", "test", "chart", "values-full.yaml")
	rendered := run(t, helm, "template", "e2e", chart, "-f", values)
	for _, required := range []string{
		"kind: ServiceMonitor", "kind: PrometheusRule",
		"kind: NetworkPolicy", "kind: PodDisruptionBudget",
	} {
		if !strings.Contains(rendered, required) {
			t.Errorf("full chart missing %q", required)
		}
	}
	if !strings.Contains(rendered, "debug:\n        enabled: false") ||
		strings.Contains(rendered, "path: /debug") {
		t.Error("full chart exposed debug configuration")
	}
	manifest := filepath.Join(t.TempDir(), "manifest.yaml")
	if err := os.WriteFile(manifest, []byte(rendered), 0o600); err != nil {
		t.Fatalf("write rendered manifest: %v", err)
	}
	summary := run(t, kubeconform, "-strict", "-summary", "-schema-location", "default",
		"-schema-location", "https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json",
		manifest)
	if !strings.Contains(summary, "Invalid: 0, Errors: 0, Skipped: 0") {
		t.Fatalf("kubeconform did not validate every resource: %s", summary)
	}
}

func TestNetworkPolicyRequiresKubeletCIDRs(t *testing.T) {
	helm := requireTool(t, "helm")
	output, err := exec.Command(
		helm, "template", "e2e", chartPath(t), "--set", "networkPolicy.enabled=true",
	).CombinedOutput()
	if err == nil || !strings.Contains(string(output), "networkPolicy.kubeletCIDRs must contain") {
		t.Fatalf("unsafe NetworkPolicy unexpectedly rendered: error=%v output=%s", err, output)
	}
}

func chartPath(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "charts", "aws-cost-exporter"))
	if err != nil {
		t.Fatalf("resolve chart path: %v", err)
	}
	return path
}

func read(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}

func run(t *testing.T, command string, arguments ...string) string {
	t.Helper()
	output, err := exec.Command(command, arguments...).CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", command, arguments, err, output)
	}
	return string(output)
}

func requireTool(t *testing.T, name string) string {
	t.Helper()
	path, err := exec.LookPath(name)
	if err != nil {
		t.Fatalf("%s is required to validate the chart", name)
	}
	return path
}
