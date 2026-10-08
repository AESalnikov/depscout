package gradle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleCatalog = `
[versions]
orbitCore = "2.4.1"
shadow = "8.1.1"

[libraries]
orbit-core = { module = "io.orbitcart.core:orbit-core", version.ref = "orbitCore" }
catalog-client = { group = "io.orbitcart.catalog", name = "catalog-client", version = "0.9.2" }

[plugins]
conventions = { id = "io.orbitcart.conventions", version.ref = "orbitCore" }
shadow = { id = "com.github.johnrengelman.shadow", version.ref = "shadow" }
`

func TestParseCatalog(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "libs.versions.toml")
	if err := os.WriteFile(path, []byte(sampleCatalog), 0o644); err != nil {
		t.Fatal(err)
	}
	refs, err := ParseCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 4 {
		t.Fatalf("got %d refs: %+v", len(refs), refs)
	}
	byAlias := map[string]CatalogRef{}
	for _, r := range refs {
		byAlias[r.Alias] = r
	}
	core := byAlias["orbit-core"]
	if core.GAV.String() != "io.orbitcart.core:orbit-core" || core.Current != "2.4.1" || core.VersionKey != "orbitCore" {
		t.Fatalf("%+v", core)
	}
	client := byAlias["catalog-client"]
	if !client.LiteralOnEntry || client.Current != "0.9.2" {
		t.Fatalf("%+v", client)
	}
	plug := byAlias["conventions"]
	if plug.Section != "plugin" || plug.GAV.Artifact != "io.orbitcart.conventions.gradle.plugin" {
		t.Fatalf("%+v", plug)
	}

	missing, err := ParseCatalog(filepath.Join(dir, "nope.toml"))
	if err != nil || missing != nil {
		t.Fatal(err, missing)
	}
}

func TestParseCatalogBadTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.toml")
	if err := os.WriteFile(path, []byte("[[[broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseCatalog(path); err == nil {
		t.Fatal("expected error")
	}
}

func TestUpdateCatalogVersions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "libs.versions.toml")
	if err := os.WriteFile(path, []byte(sampleCatalog), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpdateCatalogVersions(path, map[string]string{"orbitCore": "2.5.0"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `orbitCore = "2.5.0"`) {
		t.Fatal(string(data))
	}
	if err := UpdateCatalogVersions(path, nil); err != nil {
		t.Fatal(err)
	}
	if err := UpdateCatalogVersions(path, map[string]string{"missing": "1"}); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateCatalogEntryVersions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "libs.versions.toml")
	if err := os.WriteFile(path, []byte(sampleCatalog), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpdateCatalogEntryVersions(path, map[string]string{"catalog-client": "1.0.0"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `version = "1.0.0"`) {
		t.Fatal(string(data))
	}
	// version.ref entries must stay untouched by literal updater
	if !strings.Contains(string(data), `version.ref = "orbitCore"`) {
		t.Fatal(string(data))
	}
}

func TestDefaultCatalogPath(t *testing.T) {
	p := DefaultCatalogPath("/proj")
	if !strings.HasSuffix(p, "gradle/libs.versions.toml") && !strings.HasSuffix(p, `gradle\libs.versions.toml`) {
		t.Fatal(p)
	}
}

func TestCatalogCorners(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "libs.versions.toml")
	content := `
[versions]
ok = "1.0"
missingTarget = "9.9"

[libraries]
bad-module = { module = "onlygroup", version.ref = "ok" }
no-coords = { version.ref = "ok" }
broken-ref = { module = "com.ex:lib", version.ref = "nope" }
empty-ver = { module = "com.ex:lib2", version = "" }
int-ver = { module = "com.ex:lib3", version = 1 }

[plugins]
no-id = { version.ref = "ok" }
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	refs, err := ParseCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].Alias != "int-ver" {
		t.Fatalf("%+v", refs)
	}
	_, _ = anyString(float64(1.5))
	_, _ = anyString(true)

	if err := UpdateCatalogVersions(path, map[string]string{"ok": "2.0"}); err != nil {
		t.Fatal(err)
	}
	noVersions := filepath.Join(dir, "nover.toml")
	if err := os.WriteFile(noVersions, []byte("[libraries]\nx={module=\"a:b\",version=\"1\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpdateCatalogVersions(noVersions, map[string]string{"x": "1"}); err == nil {
		t.Fatal("expected no [versions]")
	}
	// CRLF section header
	crlf := "[versions]\r\nok = \"1\"\r\n[libraries]\r\n"
	span := catalogSectionSpan(crlf, "versions")
	if span.start < 0 {
		t.Fatal(span)
	}
}
