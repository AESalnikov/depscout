package gradle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseInlineDependencies(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "build.gradle")
	content := `
dependencies {
  implementation "com.ex:lib:1.2.3"
  implementation 'com.ex:other:2.0.0'
  implementation "com.ex:dyn:1.+"
  implementation "com.ex:latest:latest.release"
  implementation "com.ex:prop:${libVersion}"
  implementation "com.ex:lib:1.2.3"
}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	refs, err := ParseInlineDependencies(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 2 {
		t.Fatalf("got %d: %+v", len(refs), refs)
	}
	if refs[0].GAV.String() != "com.ex:lib" || refs[0].Version != "1.2.3" {
		t.Fatalf("%+v", refs[0])
	}
	if _, err := ParseInlineDependencies(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("expected error")
	}
}

func TestUpdateInlineVersions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "build.gradle")
	if err := os.WriteFile(path, []byte(`implementation "com.ex:lib:1.0.0"`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpdateInlineVersions(path, map[string]string{"com.ex:lib": "2.0.0"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `"com.ex:lib:2.0.0"`) {
		t.Fatal(string(data))
	}
	if err := UpdateInlineVersions(path, nil); err != nil {
		t.Fatal(err)
	}
	if err := UpdateInlineVersions(path, map[string]string{"com.ex:other": "1"}); err != nil {
		t.Fatal(err)
	}
}

func TestParsePluginsKotlinDSL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "build.gradle.kts")
	content := `
plugins {
    id("io.orbitcart.conventions") version "2.1.0"
    id("com.ex.p") version("1.0.0")
}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	plugins, err := ParsePlugins(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(plugins) != 2 {
		t.Fatalf("%+v", plugins)
	}
	if plugins[0].ID != "io.orbitcart.conventions" || plugins[0].Version != "2.1.0" {
		t.Fatalf("%+v", plugins[0])
	}
	if plugins[1].Version != "1.0.0" {
		t.Fatalf("%+v", plugins[1])
	}
}

func TestUpdatePluginVersionsKotlinDSL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "build.gradle.kts")
	if err := os.WriteFile(path, []byte(`plugins {
    id("com.ex.p") version "1.0.0"
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpdatePluginVersions(path, map[string]string{"com.ex.p": "1.1.0"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `version "1.1.0"`) {
		t.Fatal(string(data))
	}
}

func TestBuildScriptPathsAndIsGradleProject(t *testing.T) {
	dir := t.TempDir()
	if IsGradleProject(dir) {
		t.Fatal("empty")
	}
	if err := os.WriteFile(filepath.Join(dir, "build.gradle.kts"), []byte("plugins {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !IsGradleProject(dir) {
		t.Fatal("kts only")
	}
	paths := BuildScriptPaths(dir)
	if len(paths) != 1 {
		t.Fatal(paths)
	}
	if err := os.WriteFile(filepath.Join(dir, "build.gradle"), []byte("plugins {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	paths = BuildScriptPaths(dir)
	if len(paths) != 2 {
		t.Fatal(paths)
	}
}
