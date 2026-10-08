package gradle

import (
	"os"
	"path/filepath"
)

// BuildScriptPaths возвращает существующие build-скрипты в корне проекта
// (Groovy и/или Kotlin DSL). Порядок: build.gradle, затем build.gradle.kts.
func BuildScriptPaths(root string) []string {
	var out []string
	for _, name := range []string{"build.gradle", "build.gradle.kts"} {
		p := filepath.Join(root, name)
		if _, err := os.Stat(p); err == nil {
			out = append(out, p)
		}
	}
	return out
}

// DefaultCatalogPath — стандартный путь Version Catalog.
func DefaultCatalogPath(root string) string {
	return filepath.Join(root, "gradle", "libs.versions.toml")
}

// IsGradleProject true, если есть хотя бы один build-скрипт.
func IsGradleProject(root string) bool {
	return len(BuildScriptPaths(root)) > 0
}
