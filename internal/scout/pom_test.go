package scout

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AESalnikov/depscout/internal/pom"
	"github.com/AESalnikov/depscout/internal/report"
)

func TestRunMavenProjectApply(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repo/com/ex/lib/maven-metadata.xml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<metadata><versioning><versions><version>1.0.0</version><version>1.2.0</version></versions></versioning></metadata>`))
	})
	mux.HandleFunc("/repo/com/ex/lib/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "maven-metadata") {
			return
		}
		_, _ = w.Write([]byte(`<a href="1.2.0/">1.2.0/</a>`))
	})
	mux.HandleFunc("/repo/com/ex/utils/maven-metadata.xml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<metadata><versioning><versions><version>2.0.0</version><version>2.1.0</version></versions></versioning></metadata>`))
	})
	mux.HandleFunc("/repo/com/ex/utils/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "maven-metadata") {
			return
		}
		_, _ = w.Write([]byte(`<a href="2.1.0/">2.1.0/</a>`))
	})
	mux.HandleFunc("/repo/org/apache/maven/plugins/maven-compiler-plugin/maven-metadata.xml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<metadata><versioning><versions><version>3.11.0</version></versions></versioning></metadata>`))
	})
	mux.HandleFunc("/repo/org/apache/maven/plugins/maven-compiler-plugin/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "maven-metadata") {
			return
		}
		_, _ = w.Write([]byte(`<a href="3.11.0/">3.11.0/</a>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	root := t.TempDir()
	pomPath := filepath.Join(root, "pom.xml")
	if err := os.WriteFile(pomPath, []byte(`<?xml version="1.0"?>
<project>
  <properties>
    <lib.version>1.0.0</lib.version>
  </properties>
  <dependencies>
    <dependency>
      <groupId>com.ex</groupId>
      <artifactId>lib</artifactId>
      <version>${lib.version}</version>
    </dependency>
    <dependency>
      <groupId>com.ex</groupId>
      <artifactId>utils</artifactId>
      <version>2.0.0</version>
    </dependency>
  </dependencies>
  <build>
    <plugins>
      <plugin>
        <artifactId>maven-compiler-plugin</artifactId>
        <version>3.11.0</version>
      </plugin>
    </plugins>
  </build>
</project>
`), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Run(context.Background(), Options{
		Root:  root,
		Repos: []string{srv.URL + "/repo"},
		Apply: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	var sawProp, sawDep bool
	for _, row := range res.Rows {
		switch row.Kind {
		case pom.KindProperty:
			sawProp = true
			if row.Name == "lib.version" && row.Status != report.StatusOutdated {
				t.Fatalf("%+v", row)
			}
		case pom.KindDep:
			sawDep = true
		}
	}
	if !sawProp || !sawDep {
		t.Fatalf("%+v", res.Rows)
	}
	data, _ := os.ReadFile(pomPath)
	if !strings.Contains(string(data), "<lib.version>1.2.0</lib.version>") {
		t.Fatal(string(data))
	}
	if !strings.Contains(string(data), "<version>2.1.0</version>") {
		t.Fatal(string(data))
	}
}

func TestRunPomParseError(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "pom.xml"), []byte("<nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Run(context.Background(), Options{Root: root, Repos: []string{"https://example.com/r"}})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNeedsMavenReposPom(t *testing.T) {
	root := t.TempDir()
	if needsMavenRepos(root, Options{}) {
		t.Fatal("empty")
	}
	if err := os.WriteFile(filepath.Join(root, "pom.xml"), []byte("<project/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !needsMavenRepos(root, Options{}) {
		t.Fatal("pom")
	}
	if needsMavenRepos(root, Options{SkipPom: true}) {
		t.Fatal("skip pom")
	}
}
