package comrad

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNoLegacyProjectName(t *testing.T) {
	root := repoRootForNameContract(t)
	legacy := strings.Join([]string{"run", "grid"}, "")
	ignoredDirs := map[string]bool{
		".git":            true,
		".e2e":            true,
		".agents":         true,
		".playwright-cli": true,
		".tools":          true,
		"data":            true,
		"dist":            true,
		"node_modules":    true,
		"test-results":    true,
	}
	filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			t.Fatalf("walk %s: %v", path, err)
		}
		if entry.IsDir() && ignoredDirs[entry.Name()] {
			return filepath.SkipDir
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatalf("rel %s: %v", path, err)
		}
		if strings.Contains(strings.ToLower(filepath.ToSlash(rel)), legacy) {
			t.Errorf("legacy project name in path: %s", rel)
		}
		if entry.IsDir() {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if strings.Contains(strings.ToLower(string(body)), legacy) {
			t.Errorf("legacy project name in file: %s", rel)
		}
		return nil
	})
}

func repoRootForNameContract(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		next := filepath.Dir(dir)
		if next == dir {
			t.Fatal("go.mod not found")
		}
		dir = next
	}
}
