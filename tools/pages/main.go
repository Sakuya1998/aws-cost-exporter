package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const wikiBaseURL = "https://github.com/Sakuya1998/aws-cost-exporter/wiki/"

var wikiLinkPattern = regexp.MustCompile(regexp.QuoteMeta(wikiBaseURL) + `([A-Za-z0-9-]+)(#[A-Za-z0-9._~%!$&'()*+,;=:@/?-]+)?`)

func main() {
	source := flag.String("source", "docs/wiki", "directory containing the managed Wiki Markdown")
	destination := flag.String("destination", ".pages-docs", "directory to generate for MkDocs")
	flag.Parse()

	if err := prepare(*source, *destination); err != nil {
		fmt.Fprintf(os.Stderr, "prepare Pages documentation: %v\n", err)
		os.Exit(1)
	}
}

func prepare(source, destination string) error {
	sourcePath, err := filepath.Abs(source)
	if err != nil {
		return fmt.Errorf("resolve source: %w", err)
	}
	destinationPath, err := filepath.Abs(destination)
	if err != nil {
		return fmt.Errorf("resolve destination: %w", err)
	}
	if relativeSource, relativeErr := filepath.Rel(destinationPath, sourcePath); relativeErr == nil &&
		(relativeSource == "." || (relativeSource != ".." && !strings.HasPrefix(relativeSource, ".."+string(filepath.Separator)))) {
		return errors.New("destination contains source directory")
	}

	home, err := os.ReadFile(filepath.Join(sourcePath, "Home.md"))
	if err != nil {
		return fmt.Errorf("read required Home.md: %w", err)
	}
	entries, err := os.ReadDir(sourcePath)
	if err != nil {
		return fmt.Errorf("read source directory: %w", err)
	}

	if err := os.RemoveAll(destinationPath); err != nil {
		return fmt.Errorf("clear destination: %w", err)
	}
	if err := os.MkdirAll(destinationPath, 0o755); err != nil {
		return fmt.Errorf("create destination: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" || excluded(entry.Name()) {
			continue
		}
		content, readErr := os.ReadFile(filepath.Join(sourcePath, entry.Name()))
		if readErr != nil {
			return fmt.Errorf("read %s: %w", entry.Name(), readErr)
		}
		if writeErr := writePage(destinationPath, entry.Name(), rewriteWikiLinks(content)); writeErr != nil {
			return writeErr
		}
	}

	if err := writePage(destinationPath, "index.md", rewriteWikiLinks(home)); err != nil {
		return err
	}
	return nil
}

func excluded(name string) bool {
	switch name {
	case "README.md", "_Sidebar.md", "_Footer.md":
		return true
	default:
		return false
	}
}

func rewriteWikiLinks(content []byte) []byte {
	return wikiLinkPattern.ReplaceAllFunc(content, func(match []byte) []byte {
		value := strings.TrimPrefix(string(match), wikiBaseURL)
		slug, fragment, _ := strings.Cut(value, "#")
		if fragment != "" {
			fragment = "#" + fragment
		}
		if slug == "Home" {
			return []byte("index.md" + fragment)
		}
		return []byte(slug + ".md" + fragment)
	})
}

func writePage(destination, name string, content []byte) error {
	if err := os.WriteFile(filepath.Join(destination, name), content, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	return nil
}
