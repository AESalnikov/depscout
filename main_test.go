package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"
)

func TestRunVersionAndUsage(t *testing.T) {
	var out, errBuf bytes.Buffer
	if code := run([]string{"-version"}, &out, &errBuf); code != 0 {
		t.Fatalf("code=%d err=%s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), displayVersion()) {
		t.Fatal(out.String())
	}
	errBuf.Reset()
	if code := run([]string{"-h"}, &out, &errBuf); code != 2 {
		// ContinueOnError: -h returns ErrHelp → we return 2
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(errBuf.String(), "Использование:") {
		t.Fatal(errBuf.String())
	}
}

func TestRunNoRepos(t *testing.T) {
	t.Setenv("DEPSCOUT_REPOS", "")
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "build.gradle"), []byte("plugins {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errBuf bytes.Buffer
	code := run([]string{root}, &out, &errBuf)
	if code != 2 || !strings.Contains(errBuf.String(), "no Maven repositories") {
		t.Fatalf("%d %s", code, errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "--repos") {
		t.Fatal(errBuf.String())
	}
}

func TestRunBadRewrite(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := run([]string{"-repos", "https://x", "-rewrite-host", "bad"}, &out, &errBuf)
	if code != 2 || !strings.Contains(errBuf.String(), "rewrite-host") {
		t.Fatalf("%d %s", code, errBuf.String())
	}
}

func TestRunScoutErrorAndSuccess(t *testing.T) {
	t.Setenv("DEPSCOUT_REPOS", "")
	var out, errBuf bytes.Buffer
	code := run([]string{"-repos", "https://example.com/r", t.TempDir()}, &out, &errBuf)
	if code != 2 || !strings.Contains(errBuf.String(), "not a Gradle/Maven project") {
		t.Fatalf("%d %s", code, errBuf.String())
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "maven-metadata.xml") {
			_, _ = w.Write([]byte(`<metadata><versioning><versions><version>1.0.0</version></versions></versioning></metadata>`))
			return
		}
		if strings.Contains(r.URL.Path, "dist") {
			_, _ = w.Write([]byte(`<a href="gradle-8.0-bin.zip">x</a>`))
			return
		}
		_, _ = w.Write([]byte(`<a href="1.0.0/">1.0.0/</a>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "build.gradle"), []byte(`
plugins { id "com.ex.p" version "1.0.0" }
dependencies { implementation "com.ex:lib:${libVersion}" }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "gradle.properties"), []byte("libVersion=1.0.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wdir := filepath.Join(root, "gradle", "wrapper")
	_ = os.MkdirAll(wdir, 0o755)
	dist := strings.ReplaceAll(srv.URL+"/dist/gradle-8.0-bin.zip", ":", `\:`)
	if err := os.WriteFile(filepath.Join(wdir, "gradle-wrapper.properties"), []byte("distributionUrl="+dist+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out.Reset()
	errBuf.Reset()
	code = run([]string{"-repos", srv.URL + "/repo", root}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("%d %s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "KIND") || !strings.Contains(errBuf.String(), "dry-run") {
		t.Fatalf("out=%s err=%s", out.String(), errBuf.String())
	}

	// check with outdated: bump server metadata via separate project with old version and newer in repo
	out.Reset()
	errBuf.Reset()
	// force ERROR hint path: use unreachable repo for deps but skip wrapper/plugins...
	// Instead create error row by closed server for one artifact - use skip-wrapper and bad repo partially.
	// Simpler: --check when outdated
	mux2 := http.NewServeMux()
	mux2.HandleFunc("/repo/com/ex/lib/maven-metadata.xml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<metadata><versioning><versions><version>9.9.9</version></versions></versioning></metadata>`))
	})
	mux2.HandleFunc("/repo/com/ex/lib/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "maven-metadata") {
			return
		}
		_, _ = w.Write([]byte(`<a href="9.9.9/">9.9.9/</a>`))
	})
	mux2.HandleFunc("/repo/com/ex/p/com.ex.p.gradle.plugin/maven-metadata.xml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<metadata><versioning><versions><version>1.0.0</version></versions></versioning></metadata>`))
	})
	mux2.HandleFunc("/repo/com/ex/p/com.ex.p.gradle.plugin/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "maven-metadata") {
			return
		}
		_, _ = w.Write([]byte(`<a href="1.0.0/">1.0.0/</a>`))
	})
	srv2 := httptest.NewServer(mux2)
	defer srv2.Close()

	out.Reset()
	errBuf.Reset()
	code = run([]string{
		"-repos", srv2.URL + "/repo",
		"-skip-wrapper",
		"-check",
		root,
	}, &out, &errBuf)
	if code != 1 {
		t.Fatalf("want check exit 1, got %d err=%s out=%s", code, errBuf.String(), out.String())
	}

	out.Reset()
	errBuf.Reset()
	code = run([]string{
		"-repos", srv2.URL + "/repo",
		"-skip-wrapper",
		"-apply",
		root,
	}, &out, &errBuf)
	if code != 0 || !strings.Contains(errBuf.String(), "обновления записаны") {
		t.Fatalf("%d %s", code, errBuf.String())
	}
}

func TestRunSkipDepsPluginsNeedsNoRepos(t *testing.T) {
	t.Setenv("DEPSCOUT_REPOS", "")
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "build.gradle"), []byte("plugins {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wdir := filepath.Join(root, "gradle", "wrapper")
	_ = os.MkdirAll(wdir, 0o755)
	// wrapper will ERROR
	if err := os.WriteFile(filepath.Join(wdir, "gradle-wrapper.properties"), []byte(
		"distributionUrl=https\\://127.0.0.1:1/gradle-8.0-bin.zip\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errBuf bytes.Buffer
	code := run([]string{"-skip-deps", "-skip-plugins", root}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("%d %s", code, errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "подсказка:") {
		t.Fatalf("want error hint: %s", errBuf.String())
	}
}

func TestEprintln(t *testing.T) {
	var b bytes.Buffer
	eprintln(&b, "hello %s", "x")
	if b.String() != "hello x\n" {
		t.Fatal(b.String())
	}
}

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, context.Canceled }

func TestRunWriteTableError(t *testing.T) {
	// need import context - add to imports
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "build.gradle"), []byte("plugins {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var errBuf bytes.Buffer
	code := run([]string{"-skip-deps", "-skip-plugins", "-skip-wrapper", root}, errWriter{}, &errBuf)
	if code != 2 || !strings.Contains(errBuf.String(), "depscout:") {
		t.Fatalf("%d %s", code, errBuf.String())
	}
}

func TestMainEntrypoint(t *testing.T) {
	oldExit := osExit
	oldArgs := os.Args
	defer func() {
		osExit = oldExit
		os.Args = oldArgs
	}()
	var code int
	osExit = func(c int) { code = c }
	os.Args = []string{"depscout", "-version"}
	main()
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
}

func TestDisplayVersion(t *testing.T) {
	origVer := Version
	origRead := readBuildInfo
	t.Cleanup(func() {
		Version = origVer
		readBuildInfo = origRead
	})

	Version = "1.2.3"
	if displayVersion() != "1.2.3" {
		t.Fatal(displayVersion())
	}

	Version = "dev"
	readBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Main: debug.Module{Version: "v9.8.7"}}, true
	}
	if displayVersion() != "9.8.7" {
		t.Fatal(displayVersion())
	}

	Version = "  "
	readBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, true
	}
	if displayVersion() != "dev" {
		t.Fatal(displayVersion())
	}

	readBuildInfo = func() (*debug.BuildInfo, bool) { return nil, false }
	if displayVersion() != "dev" {
		t.Fatal(displayVersion())
	}
}
