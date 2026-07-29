package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareCopiesManagedPagesAndRewritesWikiLinks(t *testing.T) {
	source := t.TempDir()
	destination := filepath.Join(t.TempDir(), ".pages-docs")

	writeTestFile(t, source, "Home.md", "[Home](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Home)\n[Config](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Configuration-Reference#limits)\n")
	writeTestFile(t, source, "Home-zh-CN.md", "[English](https://github.com/Sakuya1998/aws-cost-exporter/wiki/Home)\n")
	writeTestFile(t, source, "Configuration-Reference.md", "# Configuration\n")
	writeTestFile(t, source, "README.md", "authoring only\n")
	writeTestFile(t, source, "_Sidebar.md", "wiki only\n")
	writeTestFile(t, source, "_Footer.md", "wiki only\n")

	if err := prepare(source, destination); err != nil {
		t.Fatalf("prepare pages: %v", err)
	}

	for _, name := range []string{"Home.md", "Home-zh-CN.md", "Configuration-Reference.md", "index.md"} {
		if _, err := os.Stat(filepath.Join(destination, name)); err != nil {
			t.Errorf("expected generated page %s: %v", name, err)
		}
	}
	for _, name := range []string{"README.md", "_Sidebar.md", "_Footer.md"} {
		if _, err := os.Stat(filepath.Join(destination, name)); !os.IsNotExist(err) {
			t.Errorf("excluded page %s exists or stat failed: %v", name, err)
		}
	}

	home := readTestFile(t, filepath.Join(destination, "Home.md"))
	for _, want := range []string{"[Home](index.md)", "[Config](Configuration-Reference.md#limits)"} {
		if !strings.Contains(home, want) {
			t.Errorf("generated Home.md lacks %q:\n%s", want, home)
		}
	}
	if strings.Contains(home, "github.com/Sakuya1998/aws-cost-exporter/wiki") {
		t.Errorf("generated Home.md retains an absolute Wiki link:\n%s", home)
	}
	if index := readTestFile(t, filepath.Join(destination, "index.md")); index != home {
		t.Errorf("index.md must be the processed Home.md\nindex:\n%s\nhome:\n%s", index, home)
	}
}

func TestPrepareReplacesStaleDestination(t *testing.T) {
	source := t.TempDir()
	destination := filepath.Join(t.TempDir(), ".pages-docs")
	writeTestFile(t, source, "Home.md", "# Home\n")
	writeTestFile(t, destination, "stale.md", "stale\n")

	if err := prepare(source, destination); err != nil {
		t.Fatalf("prepare pages: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destination, "stale.md")); !os.IsNotExist(err) {
		t.Fatalf("stale output was not removed: %v", err)
	}
}

func TestPrepareRequiresHomePage(t *testing.T) {
	source := t.TempDir()
	destination := filepath.Join(t.TempDir(), ".pages-docs")
	writeTestFile(t, source, "Installation.md", "# Installation\n")

	err := prepare(source, destination)
	if err == nil || !strings.Contains(err.Error(), "Home.md") {
		t.Fatalf("prepare error=%v, want missing Home.md error", err)
	}
}

func TestPrepareRejectsDestinationContainingSource(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "docs", "wiki")
	writeTestFile(t, source, "Home.md", "# Home\n")

	err := prepare(source, root)
	if err == nil || !strings.Contains(err.Error(), "destination contains source") {
		t.Fatalf("prepare error=%v, want destination contains source error", err)
	}
	if _, statErr := os.Stat(filepath.Join(source, "Home.md")); statErr != nil {
		t.Fatalf("source was changed after unsafe destination was rejected: %v", statErr)
	}
}

func writeTestFile(t *testing.T, directory, name, content string) {
	t.Helper()
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
