package scout

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRunDebugFlag(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "build.gradle"), []byte("plugins {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Run(context.Background(), Options{
		Root:        root,
		SkipCatalog: true,
		SkipDeps:    true,
		SkipPlugins: true,
		SkipWrapper: true,
		SkipPom:     true,
		Debug:       true,
		Repos:       []string{"https://example.com/r"},
	})
	if err != nil {
		t.Fatal(err)
	}
}
