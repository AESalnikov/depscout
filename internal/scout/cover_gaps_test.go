package scout

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AESalnikov/depscout/internal/maven"
	"github.com/AESalnikov/depscout/internal/report"
)

func TestScoutCatalogErrorViaRun(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "build.gradle"), []byte("plugins {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cat := filepath.Join(root, "bad.toml")
	if err := os.WriteFile(cat, []byte("[[[broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Run(context.Background(), Options{
		Root:        root,
		Repos:       []string{"https://example.com/r"},
		CatalogFile: cat,
		SkipDeps:    true,
		SkipPlugins: true,
		SkipWrapper: true,
		SkipPom:     true,
	})
	if err == nil {
		t.Fatal("expected catalog parse error")
	}
}

func TestScoutInlineErrorAndClaimed(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repo/com/ex/lib/maven-metadata.xml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<metadata><versioning><versions><version>2.0.0</version></versions></versioning></metadata>`))
	})
	mux.HandleFunc("/repo/com/ex/lib/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "maven-metadata") {
			return
		}
		_, _ = w.Write([]byte(`<a href="2.0.0/">2.0.0/</a>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "build.gradle"), []byte(`
dependencies {
  implementation "com.ex:lib:${libVersion}"
  implementation "com.ex:lib:1.0.0"
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "gradle.properties"), []byte("libVersion=1.0.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// catalog claims same GAV first
	catDir := filepath.Join(root, "gradle")
	_ = os.MkdirAll(catDir, 0o755)
	if err := os.WriteFile(filepath.Join(catDir, "libs.versions.toml"), []byte(`
[versions]
lib = "1.0.0"
[libraries]
lib = { module = "com.ex:lib", version.ref = "lib" }
`), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Run(context.Background(), Options{
		Root:        root,
		Repos:       []string{srv.URL + "/repo"},
		SkipPlugins: true,
		SkipWrapper: true,
		SkipPom:     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	// property+inline should be deduped away; only catalog
	for _, row := range res.Rows {
		if row.Kind == report.KindInline || row.Kind == report.KindProperty {
			t.Fatalf("should be deduped: %+v", row)
		}
	}

	// inline parse error: build.gradle is a directory name collision — use unreadable file
	root2 := t.TempDir()
	if err := os.WriteFile(filepath.Join(root2, "build.gradle"), []byte(`implementation "com.ex:x:1.0.0"`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(root2, "build.gradle"), 0); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(filepath.Join(root2, "build.gradle"), 0o644) }()
	_, err = Run(context.Background(), Options{
		Root:        root2,
		Repos:       []string{srv.URL + "/repo"},
		SkipCatalog: true,
		SkipPlugins: true,
		SkipWrapper: true,
		SkipPom:     true,
		SkipDeps:    false,
	})
	// properties missing → scoutDeps returns nil; scoutInline fails on unreadable build.gradle
	if err == nil {
		// scoutDeps: no properties file → nil; then inline fails
		t.Fatal("expected inline read error")
	}
}

func TestScoutPomClaimedAndEmpty(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<metadata><versioning><versions><version>1.0.0</version></versions></versioning></metadata>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "build.gradle"), []byte("plugins {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pom.xml"), []byte(`<?xml version="1.0"?>
<project>
  <dependencies>
    <dependency>
      <groupId>com.ex</groupId>
      <artifactId>lib</artifactId>
      <version>1.0.0</version>
    </dependency>
  </dependencies>
</project>`), 0o644); err != nil {
		t.Fatal(err)
	}
	catDir := filepath.Join(root, "gradle")
	_ = os.MkdirAll(catDir, 0o755)
	if err := os.WriteFile(filepath.Join(catDir, "libs.versions.toml"), []byte(`
[versions]
lib = "1.0.0"
[libraries]
lib = { module = "com.ex:lib", version.ref = "lib" }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Run(context.Background(), Options{
		Root:        root,
		Repos:       []string{srv.URL + "/repo"},
		SkipDeps:    true,
		SkipPlugins: true,
		SkipWrapper: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range res.Rows {
		if row.Kind == "pom-dep" {
			t.Fatalf("pom should be claimed by catalog: %+v", row)
		}
	}

	root2 := t.TempDir()
	if err := os.WriteFile(filepath.Join(root2, "pom.xml"), []byte("<project></project>"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err = Run(context.Background(), Options{Root: root2, Repos: []string{srv.URL + "/repo"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) != 0 {
		t.Fatalf("%+v", res.Rows)
	}
}

func TestApplyUpdatesPomAndInlineErrors(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "build.gradle"), []byte("plugins {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pom.xml"), []byte(samplePOMMini), 0o644); err != nil {
		t.Fatal(err)
	}
	upd := newFileUpdates()
	upd.pomProps["lib.version"] = "9"
	upd.pomLiterals["com.ex:utils"] = "9"
	if err := os.Chmod(filepath.Join(root, "pom.xml"), 0o444); err != nil {
		t.Fatal(err)
	}
	_ = os.Chmod(root, 0o555)
	defer func() {
		_ = os.Chmod(root, 0o755)
		_ = os.Chmod(filepath.Join(root, "pom.xml"), 0o644)
	}()
	if err := applyUpdates(root, "", upd); err == nil {
		t.Fatal("expected pom props error")
	}

	_ = os.Chmod(root, 0o755)
	_ = os.Chmod(filepath.Join(root, "pom.xml"), 0o644)
	upd2 := newFileUpdates()
	upd2.pomLiterals["com.ex:utils"] = "9"
	if err := os.Chmod(filepath.Join(root, "pom.xml"), 0o444); err != nil {
		t.Fatal(err)
	}
	_ = os.Chmod(root, 0o555)
	if err := applyUpdates(root, "", upd2); err == nil {
		t.Fatal("expected pom literals error")
	}
	_ = os.Chmod(root, 0o755)
	_ = os.Chmod(filepath.Join(root, "pom.xml"), 0o644)

	bp := filepath.Join(root, "build.gradle")
	if err := os.WriteFile(bp, []byte(`implementation "com.ex:x:1.0.0"`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	upd3 := newFileUpdates()
	upd3.inlineByFile[bp] = map[string]string{"com.ex:x": "2.0.0"}
	if err := os.Chmod(bp, 0); err != nil {
		t.Fatal(err)
	}
	if err := applyUpdates(root, "", upd3); err == nil {
		t.Fatal("expected inline error")
	}
	_ = os.Chmod(bp, 0o644)

	cat := filepath.Join(root, "c.toml")
	if err := os.WriteFile(cat, []byte("[versions]\nv=\"1\"\n[libraries]\ny={module=\"a:b\",version=\"1\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	upd4 := newFileUpdates()
	upd4.catalogLiterals["y"] = "2"
	if err := os.Chmod(cat, 0o444); err != nil {
		t.Fatal(err)
	}
	_ = os.Chmod(root, 0o555)
	if err := applyUpdates(root, cat, upd4); err == nil {
		t.Fatal("expected catalog literal error")
	}
}

const samplePOMMini = `<?xml version="1.0"?>
<project>
  <properties><lib.version>1.0</lib.version></properties>
  <dependencies>
    <dependency>
      <groupId>com.ex</groupId>
      <artifactId>utils</artifactId>
      <version>1.0</version>
    </dependency>
  </dependencies>
</project>`

func TestScoutPluginsEmpty(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "build.gradle"), []byte("plugins {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := maven.NewClient([]string{"https://example.com/r"}, false)
	rows, err := scoutPlugins(context.Background(), root, c, newFileUpdates())
	if err != nil || rows != nil {
		t.Fatalf("got %v err=%v", rows, err)
	}
}

func TestScoutPluginsErrorMultiScript(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "build.gradle"), []byte("plugins {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "build.gradle.kts"), []byte("plugins {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(root, "build.gradle.kts"), 0); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(filepath.Join(root, "build.gradle.kts"), 0o644) }()
	_, err := Run(context.Background(), Options{
		Root:        root,
		Repos:       []string{"https://example.com/r"},
		SkipDeps:    true,
		SkipCatalog: true,
		SkipWrapper: true,
		SkipPom:     true,
	})
	if err == nil {
		t.Fatal("expected plugins read error")
	}
}
