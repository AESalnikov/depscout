package scout

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AESalnikov/depscout/internal/report"
)

func TestRunCatalogAndInlineApply(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repo/io/orbitcart/core/orbit-core/maven-metadata.xml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<metadata><versioning><versions><version>2.4.1</version><version>2.5.0</version></versions></versioning></metadata>`))
	})
	mux.HandleFunc("/repo/io/orbitcart/core/orbit-core/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "maven-metadata") {
			return
		}
		_, _ = w.Write([]byte(`<a href="2.5.0/">2.5.0/</a>`))
	})
	mux.HandleFunc("/repo/com/ex/inline/maven-metadata.xml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<metadata><versioning><versions><version>1.0.0</version><version>1.1.0</version></versions></versioning></metadata>`))
	})
	mux.HandleFunc("/repo/com/ex/inline/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "maven-metadata") {
			return
		}
		_, _ = w.Write([]byte(`<a href="1.1.0/">1.1.0/</a>`))
	})
	mux.HandleFunc("/repo/com/ex/literal/maven-metadata.xml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<metadata><versioning><versions><version>0.1.0</version><version>0.2.0</version></versions></versioning></metadata>`))
	})
	mux.HandleFunc("/repo/com/ex/literal/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "maven-metadata") {
			return
		}
		_, _ = w.Write([]byte(`<a href="0.2.0/">0.2.0/</a>`))
	})
	mux.HandleFunc("/repo/io/orbitcart/conventions/io.orbitcart.conventions.gradle.plugin/maven-metadata.xml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<metadata><versioning><versions><version>2.4.1</version><version>2.5.0</version></versions></versioning></metadata>`))
	})
	mux.HandleFunc("/repo/io/orbitcart/conventions/io.orbitcart.conventions.gradle.plugin/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "maven-metadata") {
			return
		}
		_, _ = w.Write([]byte(`<a href="2.5.0/">2.5.0/</a>`))
	})
	mux.HandleFunc("/dist/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<a href="gradle-8.0-bin.zip">a</a>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "build.gradle"), []byte(`
plugins {
    id "com.ex.other" version "1.0.0"
}
dependencies {
    implementation "com.ex:inline:1.0.0"
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	catDir := filepath.Join(root, "gradle")
	if err := os.MkdirAll(catDir, 0o755); err != nil {
		t.Fatal(err)
	}
	catPath := filepath.Join(catDir, "libs.versions.toml")
	if err := os.WriteFile(catPath, []byte(`
[versions]
orbitCore = "2.4.1"

[libraries]
orbit-core = { module = "io.orbitcart.core:orbit-core", version.ref = "orbitCore" }
literal-lib = { group = "com.ex", name = "literal", version = "0.1.0" }

[plugins]
conventions = { id = "io.orbitcart.conventions", version.ref = "orbitCore" }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	// literal-lib will be NOT_FOUND — ok
	wdir := filepath.Join(root, "gradle", "wrapper")
	if err := os.MkdirAll(wdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wdir, "gradle-wrapper.properties"), []byte(
		"distributionUrl="+strings.ReplaceAll(srv.URL+"/dist/gradle-8.0-bin.zip", ":", `\:`)+"\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Run(context.Background(), Options{
		Root:        root,
		Repos:       []string{srv.URL + "/repo"},
		Apply:       true,
		SkipPlugins: true, // skip unreacheable com.ex.other
	})
	if err != nil {
		t.Fatal(err)
	}
	var sawCatalog, sawInline bool
	for _, row := range res.Rows {
		switch row.Kind {
		case report.KindCatalog:
			sawCatalog = true
			if strings.Contains(row.Name, "orbit-core") && row.Status != report.StatusOutdated {
				t.Fatalf("catalog orbit: %+v", row)
			}
		case report.KindInline:
			sawInline = true
			if row.Status != report.StatusOutdated {
				t.Fatalf("inline: %+v", row)
			}
		}
	}
	if !sawCatalog || !sawInline {
		t.Fatalf("rows=%+v", res.Rows)
	}
	data, _ := os.ReadFile(catPath)
	if !strings.Contains(string(data), `orbitCore = "2.5.0"`) {
		t.Fatal(string(data))
	}
	if !strings.Contains(string(data), `version = "0.2.0"`) {
		t.Fatal("literal catalog entry not updated:", string(data))
	}
	build, _ := os.ReadFile(filepath.Join(root, "build.gradle"))
	if !strings.Contains(string(build), `"com.ex:inline:1.1.0"`) {
		t.Fatal(string(build))
	}
}

func TestRunKotlinDSLProject(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repo/com/ex/p/com.ex.p.gradle.plugin/maven-metadata.xml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<metadata><versioning><versions><version>1.0.0</version></versions></versioning></metadata>`))
	})
	mux.HandleFunc("/repo/com/ex/p/com.ex.p.gradle.plugin/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "maven-metadata") {
			return
		}
		_, _ = w.Write([]byte(`<a href="1.0.0/">1.0.0/</a>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "build.gradle.kts"), []byte(`
plugins {
    id("com.ex.p") version "1.0.0"
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Run(context.Background(), Options{
		Root:        root,
		Repos:       []string{srv.URL + "/repo"},
		SkipDeps:    true,
		SkipCatalog: true,
		SkipWrapper: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) != 1 || res.Rows[0].Status != report.StatusOK {
		t.Fatalf("%+v", res.Rows)
	}
}

func TestNeedsMavenReposAndCatalogPath(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "build.gradle"), []byte("plugins {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !needsMavenRepos(root, Options{}) {
		t.Fatal("deps on")
	}
	if needsMavenRepos(root, Options{SkipDeps: true, SkipPlugins: true, SkipCatalog: true}) {
		t.Fatal("all skipped")
	}
	if needsMavenRepos(root, Options{SkipDeps: true, SkipPlugins: true}) {
		t.Fatal("no catalog file")
	}
	cat := filepath.Join(root, "custom.toml")
	if err := os.WriteFile(cat, []byte("[versions]\nv=\"1\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !needsMavenRepos(root, Options{SkipDeps: true, SkipPlugins: true, CatalogFile: cat}) {
		t.Fatal("catalog present")
	}
	if catalogPath(root, cat) != cat {
		t.Fatal(catalogPath(root, cat))
	}
}

func TestApplyCatalogUpdatesErrors(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "build.gradle"), []byte("plugins {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	upd := newFileUpdates()
	upd.catalogVersions["v"] = "2"
	if err := applyUpdates(root, "", upd); err == nil {
		t.Fatal("expected missing [versions]")
	}
	cat := filepath.Join(root, "c.toml")
	if err := os.WriteFile(cat, []byte("[versions]\nv = \"1\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(cat, 0o444); err != nil {
		t.Fatal(err)
	}
	_ = os.Chmod(root, 0o555)
	defer func() {
		_ = os.Chmod(root, 0o755)
		_ = os.Chmod(cat, 0o644)
	}()
	upd2 := newFileUpdates()
	upd2.catalogVersions["v"] = "2"
	if err := applyUpdates(root, cat, upd2); err == nil {
		t.Fatal("expected catalog write error")
	}
}
