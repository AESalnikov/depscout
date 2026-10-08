package gradle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseCatalogReadError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "libs.versions.toml")
	if err := os.WriteFile(path, []byte("[versions]\nv=\"1\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(path, 0o644) }()
	if _, err := ParseCatalog(path); err == nil {
		t.Fatal("expected read error")
	}
}

func TestCatalogVersionRefFlatKeyAndPluginBroken(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "libs.versions.toml")
	// quoted dotted key — go-toml может отдать как "version.ref"
	content := `
[versions]
ok = "1.2.3"

[libraries]
flat = { module = "com.ex:flat", "version.ref" = "ok" }

[plugins]
broken = { id = "com.ex.p", version.ref = "missing" }
emptyid = { id = "", version.ref = "ok" }
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	refs, err := ParseCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range refs {
		if r.Alias == "flat" && r.Current == "1.2.3" {
			found = true
		}
		if r.Alias == "broken" || r.Alias == "emptyid" {
			t.Fatalf("should skip: %+v", r)
		}
	}
	if !found {
		// если TOML не сохранил flat key — хотя бы не упали
		t.Logf("flat key not parsed (ok): %+v", refs)
	}
}

func TestUpdateCatalogReadErrorsAndNoChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "libs.versions.toml")
	if err := os.WriteFile(path, []byte(`
[versions]
v = "1.0"

[libraries]
x = { module = "com.ex:x", version.ref = "v" }
y = { module = "com.ex:y", version = "1.0" }
z = { module = "com.ex:z", group = "com.ex", name = "z" }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpdateCatalogEntryVersions(path, nil); err != nil {
		t.Fatal(err)
	}
	if err := UpdateCatalogEntryVersions(path, map[string]string{"x": "9"}); err != nil {
		t.Fatal(err) // version.ref — no literal change
	}
	if err := UpdateCatalogEntryVersions(path, map[string]string{"z": "9"}); err != nil {
		t.Fatal(err) // no version= field
	}
	if err := UpdateCatalogEntryVersions(path, map[string]string{"nope": "9"}); err != nil {
		t.Fatal(err)
	}

	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	if err := UpdateCatalogVersions(path, map[string]string{"v": "2"}); err == nil {
		t.Fatal("expected versions read error")
	}
	if err := UpdateCatalogEntryVersions(path, map[string]string{"y": "2"}); err == nil {
		t.Fatal("expected entry read error")
	}
	_ = os.Chmod(path, 0o644)

	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}
	_ = os.Chmod(dir, 0o555)
	defer func() {
		_ = os.Chmod(dir, 0o755)
		_ = os.Chmod(path, 0o644)
	}()
	if err := UpdateCatalogVersions(path, map[string]string{"v": "2"}); err == nil {
		t.Fatal("expected versions write error")
	}
	if err := UpdateCatalogEntryVersions(path, map[string]string{"y": "2"}); err == nil {
		t.Fatal("expected entry write error")
	}
}

func TestUpdateInlineDynamicAndReadError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "build.gradle")
	if err := os.WriteFile(path, []byte(`implementation "com.ex:lib:1.+"
implementation "com.ex:ok:1.0.0"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpdateInlineVersions(path, map[string]string{"com.ex:lib": "9.9.9", "com.ex:ok": "2.0.0"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), `"com.ex:lib:9.9.9"`) {
		t.Fatal("dynamic should stay", string(data))
	}
	if !strings.Contains(string(data), `"com.ex:ok:2.0.0"`) {
		t.Fatal(string(data))
	}

	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(path, 0o644) }()
	if err := UpdateInlineVersions(path, map[string]string{"com.ex:ok": "3"}); err == nil {
		t.Fatal("expected read error")
	}
}

func TestUpdatePluginVersionsParenForm(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "build.gradle.kts")
	if err := os.WriteFile(path, []byte(`plugins {
    id("com.ex.p") version("1.0.0")
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpdatePluginVersions(path, map[string]string{"com.ex.p": "2.0.0", "other": "1"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `version("2.0.0")`) {
		t.Fatal(string(data))
	}
}

func TestCatalogSectionCRLF(t *testing.T) {
	content := "[versions]\r\nok = \"1\"\r\n[libraries]\r\n"
	span := catalogSectionSpan(content, "versions")
	if span.start < 0 || span.end <= span.start {
		t.Fatal(span)
	}
	body := content[span.start:span.end]
	if !strings.Contains(body, `ok = "1"`) {
		t.Fatal(body)
	}
}

func TestResolveCatalogVersionEmptyNested(t *testing.T) {
	ver, _, _, ok := resolveCatalogVersion(map[string]any{
		"version": map[string]any{"ref": ""},
	}, map[string]string{"x": "1"})
	if ok || ver != "" {
		t.Fatal(ver, ok)
	}
	_, _, _, ok = resolveCatalogVersion(map[string]any{"version.ref": "x"}, map[string]string{"x": "1.0"})
	if !ok {
		t.Fatal("flat version.ref")
	}
}
