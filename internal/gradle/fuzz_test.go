package gradle

import (
	"os"
	"path/filepath"
	"testing"
)

func FuzzParseBuildScriptSnippets(f *testing.F) {
	f.Add([]byte(`plugins { id "x.y" version "1.0.0" }
dependencies { implementation "g:a:${v}" implementation "g:b:1.2.3" }`))
	f.Add([]byte(`plugins { id("x.y") version("1.0.0") }`))
	f.Add([]byte(``))
	f.Fuzz(func(t *testing.T, data []byte) {
		dir := t.TempDir()
		path := filepath.Join(dir, "build.gradle")
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		_, _ = ParseDependencyProps(path)
		_, _ = ParseInlineDependencies(path)
		_, _ = ParsePlugins(path)
	})
}

func FuzzParseCatalogTOML(f *testing.F) {
	f.Add([]byte(`[versions]
x = "1.0.0"
[libraries]
lib = { module = "g:a", version.ref = "x" }
`))
	f.Add([]byte(`not toml`))
	f.Fuzz(func(t *testing.T, data []byte) {
		dir := t.TempDir()
		path := filepath.Join(dir, "libs.versions.toml")
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		_, _ = ParseCatalog(path)
	})
}

func FuzzParseWrapperProperties(f *testing.F) {
	f.Add([]byte(`distributionUrl=https\://example.com/gradle-8.12.1-bin.zip
`))
	f.Add([]byte(`distributionUrl=broken`))
	f.Fuzz(func(t *testing.T, data []byte) {
		dir := t.TempDir()
		path := filepath.Join(dir, "gradle-wrapper.properties")
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		_, _ = ParseWrapper(path)
	})
}
