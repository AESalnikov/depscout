package gradle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testdata(t *testing.T, parts ...string) string {
	t.Helper()
	root := filepath.Join("..", "..", "testdata", "sample")
	return filepath.Join(append([]string{root}, parts...)...)
}

func TestParseProperties(t *testing.T) {
	vals, order, err := ParseProperties(testdata(t, "gradle.properties"))
	if err != nil {
		t.Fatal(err)
	}
	if vals["orbitCoreVersion"] != "2.4.1" {
		t.Fatalf("orbitCoreVersion=%q", vals["orbitCoreVersion"])
	}
	if vals["cache.ttl.hours"] != "24" {
		t.Fatalf("cache.ttl.hours=%q", vals["cache.ttl.hours"])
	}
	if !IsVersionProperty("orbitCoreVersion", vals["orbitCoreVersion"]) {
		t.Fatal("expected version property")
	}
	if IsVersionProperty("org.gradle.parallel", vals["org.gradle.parallel"]) {
		t.Fatal("org.gradle.parallel should be skipped")
	}
	if len(order) == 0 {
		t.Fatal("empty order")
	}
}

func TestParsePropertiesCorners(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gradle.properties")
	content := `
# comment
! bang comment

libVersion=1.0.0
dup=1
dup=2
colonKey:3.0
noequals
=novalue
 emptyKey = 1
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	vals, order, err := ParseProperties(path)
	if err != nil {
		t.Fatal(err)
	}
	if vals["dup"] != "2" {
		t.Fatalf("last wins: %q", vals["dup"])
	}
	if vals["colonKey"] != "3.0" {
		t.Fatalf("colon: %q", vals["colonKey"])
	}
	if len(order) < 2 {
		t.Fatalf("order=%v", order)
	}

	if _, _, err := ParseProperties(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestSplitProp(t *testing.T) {
	if _, _, ok := splitProp(""); ok {
		t.Fatal("empty")
	}
	if _, _, ok := splitProp("=x"); ok {
		t.Fatal("empty key idx")
	}
	if _, _, ok := splitProp(" =x"); ok {
		t.Fatal("empty key after trim")
	}
	k, v, ok := splitProp("a=b")
	if !ok || k != "a" || v != "b" {
		t.Fatalf("%s=%s ok=%v", k, v, ok)
	}
}

func TestParsePropertiesScannerErr(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := ParseProperties(dir); err == nil {
		t.Fatal("expected scanner error on directory")
	}
}

func TestIsVersionPropertyCorners(t *testing.T) {
	cases := []struct {
		key, val string
		want     bool
	}{
		{"", "1", false},
		{"k", "", false},
		{"org.gradle.x", "1", false},
		{"flag", "true", false},
		{"flag", "FALSE", false},
		{"desc", "hello world", false},
		{"v", "abc", false},
		{"v", "1.2.3-SNAPSHOT", true},
		{"v", "1.2.3+meta", true},
	}
	for _, tc := range cases {
		if got := IsVersionProperty(tc.key, tc.val); got != tc.want {
			t.Fatalf("%s=%s: got %v want %v", tc.key, tc.val, got, tc.want)
		}
	}
}

func TestParseDependencyProps(t *testing.T) {
	refs, err := ParseDependencyProps(testdata(t, "build.gradle"))
	if err != nil {
		t.Fatal(err)
	}
	by := FirstGAVByProperty(refs)
	g, ok := by["orbitCoreVersion"]
	if !ok {
		t.Fatal("orbitCoreVersion not mapped")
	}
	if g.String() != "io.orbitcart.bom:orbit-bom" {
		t.Fatalf("got %s (first match should be BOM platform)", g)
	}
	if _, ok := by["catalogClientVersion"]; !ok {
		t.Fatal("catalogClientVersion missing")
	}
}

func TestParseDependencyPropsCorners(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "build.gradle")
	content := `
dependencies {
  implementation "com.ex:lib:${libVersion}"
  implementation 'com.ex:lib:${libVersion}'
  implementation "com.ex:other:${otherVersion}"
}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	refs, err := ParseDependencyProps(path)
	if err != nil {
		t.Fatal(err)
	}
	by := FirstGAVByProperty(refs)
	if by["libVersion"].Artifact != "lib" {
		t.Fatalf("%+v", by)
	}
	if len(refs) != 2 {
		t.Fatalf("want 2 unique refs, got %d", len(refs))
	}
	if _, err := ParseDependencyProps(filepath.Join(dir, "nope")); err == nil {
		t.Fatal("expected error")
	}
}

func TestFirstGAVByPropertyFirstWins(t *testing.T) {
	refs := []DependencyRef{
		{Property: "v", GAV: GAV{Group: "a", Artifact: "one"}},
		{Property: "v", GAV: GAV{Group: "a", Artifact: "two"}},
	}
	by := FirstGAVByProperty(refs)
	if by["v"].Artifact != "one" {
		t.Fatalf("%+v", by)
	}
}

func TestParsePlugins(t *testing.T) {
	plugins, err := ParsePlugins(testdata(t, "build.gradle"))
	if err != nil {
		t.Fatal(err)
	}
	if len(plugins) != 4 {
		t.Fatalf("want 4 plugins, got %d", len(plugins))
	}
	if plugins[0].ID != "io.orbitcart.conventions" || plugins[0].Version != "2.1.0" {
		t.Fatalf("first plugin: %+v", plugins[0])
	}
	want := "org.jetbrains.kotlin.jvm:org.jetbrains.kotlin.jvm.gradle.plugin"
	if plugins[3].GAV.String() != want {
		t.Fatalf("marker=%s", plugins[3].GAV)
	}
}

func TestParsePluginsCorners(t *testing.T) {
	dir := t.TempDir()
	noPlugins := filepath.Join(dir, "build.gradle")
	if err := os.WriteFile(noPlugins, []byte("dependencies {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plugins, err := ParsePlugins(noPlugins)
	if err != nil || plugins != nil {
		t.Fatalf("want nil, got %v err=%v", plugins, err)
	}

	dup := filepath.Join(dir, "dup.gradle")
	if err := os.WriteFile(dup, []byte(`plugins {
  id "com.ex.p" version "1.0"
  id "com.ex.p" version "2.0"
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	plugins, err = ParsePlugins(dup)
	if err != nil {
		t.Fatal(err)
	}
	if len(plugins) != 1 || plugins[0].Version != "1.0" {
		t.Fatalf("%+v", plugins)
	}

	if _, err := ParsePlugins(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseWrapper(t *testing.T) {
	info, err := ParseWrapper(testdata(t, "gradle", "wrapper", "gradle-wrapper.properties"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Version != "8.12.1" {
		t.Fatalf("version=%s", info.Version)
	}
	if info.Classifier != "bin" {
		t.Fatalf("classifier=%s", info.Classifier)
	}
	wantBase := "https://packages.orbitcart.dev/maven/gradle-distributions"
	if info.BaseURL != wantBase {
		t.Fatalf("base=%s", info.BaseURL)
	}
	if WrapperPropertiesPath("/proj") != filepath.Join("/proj", "gradle", "wrapper", "gradle-wrapper.properties") {
		t.Fatal("WrapperPropertiesPath")
	}
}

func TestParseWrapperCorners(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing")
	if _, err := ParseWrapper(missing); err == nil {
		t.Fatal("missing file")
	}
	noURL := filepath.Join(dir, "p.properties")
	if err := os.WriteFile(noURL, []byte("x=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseWrapper(noURL); err == nil {
		t.Fatal("no distributionUrl")
	}
	badZip := filepath.Join(dir, "bad.properties")
	if err := os.WriteFile(badZip, []byte("distributionUrl=https\\://example.com/not-a-gradle.zip\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseWrapper(badZip); err == nil {
		t.Fatal("bad zip name")
	}
	allZip := filepath.Join(dir, "all.properties")
	if err := os.WriteFile(allZip, []byte("distributionUrl=https\\://h/gradle-8.0-all.zip\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := ParseWrapper(allZip)
	if err != nil {
		t.Fatal(err)
	}
	if info.Classifier != "all" || info.Version != "8.0" {
		t.Fatalf("%+v", info)
	}
}

func TestUpdatePropertiesValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gradle.properties")
	content := "orbitCoreVersion=2.4.1\norg.gradle.parallel=true\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpdatePropertiesValues(path, map[string]string{"orbitCoreVersion": "2.5.0"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	want := "orbitCoreVersion=2.5.0\norg.gradle.parallel=true\n"
	if string(data) != want {
		t.Fatalf("got %q", data)
	}
}

func TestUpdatePropertiesValuesCorners(t *testing.T) {
	if err := UpdatePropertiesValues("x", nil); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "p.properties")
	content := "# c\n\nlib:1.0\n  spaced=1\nbogus\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpdatePropertiesValues(path, map[string]string{"missing": "9"}); err != nil {
		t.Fatal(err)
	}
	if err := UpdatePropertiesValues(path, map[string]string{"lib": "2.0", "spaced": "2"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	s := string(data)
	if !strings.Contains(s, "lib:2.0") || !strings.Contains(s, "spaced=2") {
		t.Fatalf("%q", s)
	}
	if err := UpdatePropertiesValues(filepath.Join(dir, "no"), map[string]string{"a": "1"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestUpdatePluginVersions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "build.gradle")
	content := `plugins {
    id "io.orbitcart.openapi" version "1.4.2"
    id "io.orbitcart.conventions" version "2.1.0"
}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	err := UpdatePluginVersions(path, map[string]string{
		"io.orbitcart.conventions": "2.2.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	s := string(data)
	if !strings.Contains(s, `id "io.orbitcart.conventions" version "2.2.0"`) {
		t.Fatalf("plugin not updated: %s", s)
	}
	if !strings.Contains(s, `id "io.orbitcart.openapi" version "1.4.2"`) {
		t.Fatalf("other plugin changed: %s", s)
	}
}

func TestUpdatePluginVersionsCorners(t *testing.T) {
	if err := UpdatePluginVersions("x", nil); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "build.gradle")
	if err := os.WriteFile(path, []byte("dependencies {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpdatePluginVersions(path, map[string]string{"x": "1"}); err == nil {
		t.Fatal("expected no plugins block error")
	}
	path2 := filepath.Join(dir, "p2.gradle")
	if err := os.WriteFile(path2, []byte(`plugins {
    id "com.ex.p" version "1.0"
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpdatePluginVersions(path2, map[string]string{"other": "2"}); err != nil {
		t.Fatal(err)
	}
	if err := UpdatePluginVersions(filepath.Join(dir, "missing"), map[string]string{"a": "1"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestUpdateWrapperVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gradle-wrapper.properties")
	content := "distributionUrl=https\\://packages.orbitcart.dev/maven/gradle-distributions/gradle-8.12.1-bin.zip\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpdateWrapperVersion(path, "8.13.0"); err != nil {
		t.Fatal(err)
	}
	info, err := ParseWrapper(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Version != "8.13.0" {
		t.Fatalf("version=%s", info.Version)
	}
	if err := UpdateWrapperVersion(path, "8.13.0"); err != nil {
		t.Fatal(err)
	}
	if err := UpdateWrapperVersion(filepath.Join(dir, "missing"), "1"); err == nil {
		t.Fatal("expected error")
	}
	bad := filepath.Join(dir, "bad.properties")
	if err := os.WriteFile(bad, []byte("x=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpdateWrapperVersion(bad, "1.0"); err == nil {
		t.Fatal("expected parse error")
	}
}
