package gradle

import (
	"path/filepath"
	"runtime"
	"testing"
)

func goldenDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("Caller")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "golden")
}

func TestGoldenParseBuildScript(t *testing.T) {
	path := filepath.Join(goldenDir(t), "build.gradle")

	deps, err := ParseDependencyProps(path)
	if err != nil {
		t.Fatal(err)
	}
	wantDeps := []DependencyRef{
		{Property: "orbitCoreVersion", GAV: GAV{Group: "io.orbitcart.core", Artifact: "orbit-core"}},
		{Property: "orbitEventsVersion", GAV: GAV{Group: "io.orbitcart.events", Artifact: "orbit-events"}},
		{Property: "orbitCoreVersion", GAV: GAV{Group: "io.orbitcart.testkit", Artifact: "orbit-testkit"}},
	}
	if len(deps) != len(wantDeps) {
		t.Fatalf("deps got %d want %d: %+v", len(deps), len(wantDeps), deps)
	}
	for i := range wantDeps {
		if deps[i] != wantDeps[i] {
			t.Fatalf("deps[%d]=%+v want %+v", i, deps[i], wantDeps[i])
		}
	}

	inline, err := ParseInlineDependencies(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(inline) != 1 || inline[0].GAV.String() != "com.ex:inline-lib" || inline[0].Version != "1.0.0" {
		t.Fatalf("%+v", inline)
	}

	plugins, err := ParsePlugins(path)
	if err != nil {
		t.Fatal(err)
	}
	wantPlugins := []struct {
		id, ver, gav string
	}{
		{"io.orbitcart.conventions", "2.1.0", "io.orbitcart.conventions:io.orbitcart.conventions.gradle.plugin"},
		{"io.orbitcart.openapi", "1.4.2", "io.orbitcart.openapi:io.orbitcart.openapi.gradle.plugin"},
		{"com.github.johnrengelman.shadow", "8.1.1", "com.github.johnrengelman.shadow:com.github.johnrengelman.shadow.gradle.plugin"},
	}
	if len(plugins) != len(wantPlugins) {
		t.Fatalf("plugins got %d: %+v", len(plugins), plugins)
	}
	for i, w := range wantPlugins {
		if plugins[i].ID != w.id || plugins[i].Version != w.ver || plugins[i].GAV.String() != w.gav {
			t.Fatalf("plugin[%d]=%+v want %v", i, plugins[i], w)
		}
	}
}

func TestGoldenParseCatalog(t *testing.T) {
	path := filepath.Join(goldenDir(t), "libs.versions.toml")
	refs, err := ParseCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	byAlias := map[string]CatalogRef{}
	for _, r := range refs {
		byAlias[r.Alias] = r
	}
	core := byAlias["orbit-core"]
	if core.GAV.String() != "io.orbitcart.core:orbit-core" || core.Current != "2.4.1" || core.VersionKey != "orbitCore" {
		t.Fatalf("%+v", core)
	}
	metrics := byAlias["metrics-starter"]
	if metrics.Current != "3.1.0" || !metrics.LiteralOnEntry {
		t.Fatalf("%+v", metrics)
	}
	conv := byAlias["conventions"]
	if conv.Section != "plugin" || conv.Current != "2.4.1" {
		t.Fatalf("%+v", conv)
	}
}

func TestGoldenParseWrapper(t *testing.T) {
	path := filepath.Join(goldenDir(t), "gradle-wrapper.properties")
	info, err := ParseWrapper(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Version != "8.12.1" || info.Classifier != "bin" {
		t.Fatalf("%+v", info)
	}
	if info.BaseURL != "https://packages.orbitcart.dev/maven/gradle-distributions" {
		t.Fatalf("base=%q", info.BaseURL)
	}
}
