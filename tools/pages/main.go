package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
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

	home, pages, err := readSourcePages(sourcePath)
	if err != nil {
		return err
	}

	if err := os.RemoveAll(destinationPath); err != nil {
		return fmt.Errorf("clear destination: %w", err)
	}
	if err := os.MkdirAll(destinationPath, 0o750); err != nil {
		return fmt.Errorf("create destination: %w", err)
	}

	for _, page := range pages {
		if writeErr := writePage(destinationPath, page.name, rewriteWikiLinks(page.content)); writeErr != nil {
			return writeErr
		}
	}

	if err := writePage(destinationPath, "index.md", rewriteWikiLinks(home)); err != nil {
		return err
	}
	return nil
}

type sourcePage struct {
	name    string
	content []byte
}

func readSourcePages(sourcePath string) (home []byte, pages []sourcePage, returnErr error) {
	sourceRoot, err := os.OpenRoot(sourcePath)
	if err != nil {
		return nil, nil, fmt.Errorf("open source directory: %w", err)
	}
	defer func() {
		if closeErr := sourceRoot.Close(); returnErr == nil && closeErr != nil {
			returnErr = fmt.Errorf("close source directory: %w", closeErr)
		}
	}()

	sourceFS := sourceRoot.FS()
	home, err = fs.ReadFile(sourceFS, "Home.md")
	if err != nil {
		return nil, nil, fmt.Errorf("read required Home.md: %w", err)
	}
	entries, err := fs.ReadDir(sourceFS, ".")
	if err != nil {
		return nil, nil, fmt.Errorf("read source directory: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" || excluded(entry.Name()) {
			continue
		}
		content, readErr := fs.ReadFile(sourceFS, entry.Name())
		if readErr != nil {
			return nil, nil, fmt.Errorf("read %s: %w", entry.Name(), readErr)
		}
		pages = append(pages, sourcePage{name: entry.Name(), content: content})
	}
	return home, pages, nil
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
	if err := os.WriteFile(filepath.Join(destination, name), content, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	return nil
}
