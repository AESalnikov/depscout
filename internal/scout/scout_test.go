package scout

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AESalnikov/depscout/internal/gradle"
	"github.com/AESalnikov/depscout/internal/maven"
	"github.com/AESalnikov/depscout/internal/report"
)

func writeProject(t *testing.T, root string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "build.gradle"), []byte(`
plugins {
    id "com.ex.plugin" version "1.0.0"
}
dependencies {
    implementation "com.ex:lib:${libVersion}"
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "gradle.properties"), []byte(`
libVersion=1.0.0
tomcat.version=11.0.0
org.gradle.parallel=true
`), 0o644); err != nil {
		t.Fatal(err)
	}
	wdir := filepath.Join(root, "gradle", "wrapper")
	if err := os.MkdirAll(wdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wdir, "gradle-wrapper.properties"), []byte(
		"distributionUrl=https\\://example.com/dist/gradle-8.0-bin.zip\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}
}

func startRepo(t *testing.T) *httptest.Server {
	t.Helper()
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
	mux.HandleFunc("/repo/com/ex/plugin/com.ex.plugin.gradle.plugin/maven-metadata.xml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<metadata><versioning><versions><version>1.0.0</version><version>1.1.0</version></versions></versioning></metadata>`))
	})
	mux.HandleFunc("/repo/com/ex/plugin/com.ex.plugin.gradle.plugin/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "maven-metadata") {
			return
		}
		_, _ = w.Write([]byte(`<a href="1.1.0/">1.1.0/</a>`))
	})
	mux.HandleFunc("/dist/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<a href="gradle-8.0-bin.zip">a</a><a href="gradle-8.5-bin.zip">b</a>`))
	})
	return httptest.NewServer(mux)
}

func TestRunDryAndApply(t *testing.T) {
	srv := startRepo(t)
	defer srv.Close()

	root := t.TempDir()
	writeProject(t, root)

	// rewrite wrapper host example.com -> httptest host via HostRewrites won't map full URL easily;
	// put distributionUrl pointing at test server
	wpath := gradle.WrapperPropertiesPath(root)
	if err := os.WriteFile(wpath, []byte(
		"distributionUrl="+strings.ReplaceAll(srv.URL+"/dist/gradle-8.0-bin.zip", ":", `\:`)+"\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}

	mapPath := filepath.Join(root, "map.txt")
	if err := os.WriteFile(mapPath, []byte("tomcat.version=org.apache.tomcat:tomcat\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// tomcat won't be in repo → NOT_FOUND

	res, err := Run(context.Background(), Options{
		Root:    root,
		Repos:   []string{srv.URL + "/repo"},
		MapFile: mapPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.CountOutdated(res.Rows) < 2 {
		t.Fatalf("rows=%+v", res.Rows)
	}

	_, err = Run(context.Background(), Options{
		Root:  root,
		Repos: []string{srv.URL + "/repo"},
		Apply: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	props, _, err := gradle.ParseProperties(filepath.Join(root, "gradle.properties"))
	if err != nil {
		t.Fatal(err)
	}
	if props["libVersion"] != "1.2.0" {
		t.Fatalf("props %v", props)
	}
	plugins, err := gradle.ParsePlugins(filepath.Join(root, "build.gradle"))
	if err != nil {
		t.Fatal(err)
	}
	if plugins[0].Version != "1.1.0" {
		t.Fatalf("plugin %v", plugins)
	}
	info, err := gradle.ParseWrapper(wpath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Version != "8.5" {
		t.Fatalf("wrapper %s", info.Version)
	}
}

func TestRunCorners(t *testing.T) {
	if _, err := Run(context.Background(), Options{Root: t.TempDir()}); err == nil {
		t.Fatal("missing build.gradle")
	}
	root := t.TempDir()
	writeProject(t, root)
	if _, err := Run(context.Background(), Options{Root: root}); err == nil {
		t.Fatal("no repos")
	}

	t.Setenv("DEPSCOUT_REWRITE_HOST", "bad")
	_, err := Run(context.Background(), Options{Root: root, Repos: []string{"https://x"}, SkipDeps: true, SkipPlugins: true, SkipWrapper: true})
	if err == nil {
		t.Fatal("bad rewrite env")
	}
	t.Setenv("DEPSCOUT_REWRITE_HOST", "")

	// skip all
	res, err := Run(context.Background(), Options{
		Root:        root,
		SkipDeps:    true,
		SkipPlugins: true,
		SkipWrapper: true,
	})
	if err != nil || len(res.Rows) != 0 {
		t.Fatalf("%+v %v", res, err)
	}

	// wrapper error (bad distribution host)
	res, err = Run(context.Background(), Options{
		Root:        root,
		Repos:       []string{"https://127.0.0.1:1/repo"},
		SkipDeps:    true,
		SkipPlugins: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) != 1 || res.Rows[0].Status != report.StatusError {
		t.Fatalf("%+v", res.Rows)
	}

	// no gradle.properties
	root2 := t.TempDir()
	if err := os.WriteFile(filepath.Join(root2, "build.gradle"), []byte("plugins {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = Run(context.Background(), Options{
		Root:        root2,
		Repos:       []string{"https://example.com/r"},
		SkipPlugins: true,
		SkipWrapper: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// HostRewrites nil + empty map merge
	_, err = Run(context.Background(), Options{
		Root:         root,
		Repos:        []string{"https://example.com/r"},
		SkipDeps:     true,
		SkipPlugins:  true,
		SkipWrapper:  true,
		HostRewrites: nil,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestLoadMapFileAndApplyResolve(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "map.txt")
	if err := os.WriteFile(path, []byte(`
# c
tomcat.version=org.apache.tomcat:tomcat

`), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := loadMapFile(path)
	if err != nil || m["tomcat.version"].Artifact != "tomcat" {
		t.Fatalf("%v %v", m, err)
	}
	if _, err := loadMapFile(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("missing")
	}
	bad := filepath.Join(dir, "bad.txt")
	if err := os.WriteFile(bad, []byte("nocolon\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadMapFile(bad); err == nil {
		t.Fatal("bad line")
	}
	bad2 := filepath.Join(dir, "bad2.txt")
	if err := os.WriteFile(bad2, []byte("a=onlyone\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadMapFile(bad2); err == nil {
		t.Fatal("bad coord")
	}

	row := report.Row{Source: "g:a"}
	applyResolve(&row, maven.ResolveResult{Err: context.Canceled, Found: false}, "1", func(string) {})
	if row.Status != report.StatusError || row.Latest == "" || row.Source != "g:a" {
		t.Fatalf("%+v", row)
	}
	row = report.Row{Source: "g:a"}
	applyResolve(&row, maven.ResolveResult{Found: false}, "1", func(string) {})
	if row.Status != report.StatusNotFound {
		t.Fatal(row.Status)
	}
	row = report.Row{Source: "g:a"}
	var got string
	applyResolve(&row, maven.ResolveResult{Found: true, Latest: "2.0"}, "1.0", func(l string) { got = l })
	if row.Status != report.StatusOutdated || got != "2.0" {
		t.Fatal(row, got)
	}
	row = report.Row{Source: "g:a"}
	applyResolve(&row, maven.ResolveResult{Found: true, Latest: "1.0"}, "1.0", func(string) {})
	if row.Status != report.StatusOK {
		t.Fatal(row.Status)
	}
}

func TestApplyUpdatesEmpty(t *testing.T) {
	if err := applyUpdates(t.TempDir(), "", newFileUpdates()); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureProject(t *testing.T) {
	if err := ensureProject(t.TempDir()); err == nil {
		t.Fatal("expected error")
	}
}

func TestRunAbsError(t *testing.T) {
	old := absPath
	defer func() { absPath = old }()
	absPath = func(string) (string, error) { return "", context.Canceled }
	if _, err := Run(context.Background(), Options{Root: "."}); err == nil {
		t.Fatal("expected abs error")
	}
}

func TestScoutDepsErrors(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "build.gradle"), []byte("plugins {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// unreadable properties
	pp := filepath.Join(root, "gradle.properties")
	if err := os.WriteFile(pp, []byte("libVersion=1.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(pp, 0); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(pp, 0o644) }()
	c := maven.NewClient(nil, false)
	if _, err := scoutDeps(context.Background(), root, c, "", map[string]bool{}, newFileUpdates()); err == nil {
		t.Fatal("unreadable properties")
	}

	root2 := t.TempDir()
	if err := os.Mkdir(filepath.Join(root2, "build.gradle"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root2, "gradle.properties"), []byte("v=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := scoutDeps(context.Background(), root2, c, "", map[string]bool{}, newFileUpdates()); err == nil {
		t.Fatal("build.gradle is dir")
	}

	root3 := t.TempDir()
	writeProject(t, root3)
	badMap := filepath.Join(root3, "bad.map")
	if err := os.WriteFile(badMap, []byte("badline\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := scoutDeps(context.Background(), root3, c, badMap, map[string]bool{}, newFileUpdates()); err == nil {
		t.Fatal("bad map")
	}
}

func TestScoutPluginsAndWrapperErrors(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "build.gradle"), 0o755); err != nil {
		t.Fatal(err)
	}
	c := maven.NewClient(nil, false)
	if _, err := scoutPlugins(context.Background(), root, c, newFileUpdates()); err == nil {
		t.Fatal("plugins parse error")
	}

	root2 := t.TempDir()
	writeProject(t, root2)
	wpath := gradle.WrapperPropertiesPath(root2)
	if err := os.WriteFile(wpath, []byte("nope=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := scoutWrapper(context.Background(), root2, c, false, nil); err == nil {
		t.Fatal("wrapper parse error")
	}
	// OK non-outdated path
	srv := startRepo(t)
	defer srv.Close()
	if err := os.WriteFile(wpath, []byte(
		"distributionUrl="+strings.ReplaceAll(srv.URL+"/dist/gradle-8.5-bin.zip", ":", `\:`)+"\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}
	c2 := &maven.Client{HTTP: srv.Client()}
	row, newVer, err := scoutWrapper(context.Background(), root2, c2, false, nil)
	if err != nil || newVer != "" || row.Status != report.StatusOK {
		t.Fatalf("%+v %q %v", row, newVer, err)
	}
}

func TestRunPropagatesInnerErrors(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "gradle.properties"), []byte("libVersion=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "build.gradle"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(context.Background(), Options{
		Root:        root,
		Repos:       []string{"https://example.com/r"},
		SkipPlugins: true,
		SkipWrapper: true,
	}); err == nil {
		t.Fatal("expected deps error")
	}

	root2 := t.TempDir()
	if err := os.WriteFile(filepath.Join(root2, "build.gradle"), []byte("plugins {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root2, "gradle.properties"), 0o755); err != nil {
		t.Fatal(err) // will make ParseProperties fail via scanner
	}
	// SkipDeps false - properties is dir
	if _, err := Run(context.Background(), Options{
		Root:        root2,
		Repos:       []string{"https://example.com/r"},
		SkipPlugins: true,
		SkipWrapper: true,
	}); err == nil {
		t.Fatal("expected properties error")
	}

	root3 := t.TempDir()
	writeProject(t, root3)
	if err := os.Chmod(filepath.Join(root3, "build.gradle"), 0); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(filepath.Join(root3, "build.gradle"), 0o644) }()
	if _, err := Run(context.Background(), Options{
		Root:        root3,
		Repos:       []string{"https://example.com/r"},
		SkipDeps:    true,
		SkipWrapper: true,
	}); err == nil {
		t.Fatal("expected plugins error")
	}
}

func TestApplyUpdatesErrors(t *testing.T) {
	root := t.TempDir()
	writeProject(t, root)
	pp := filepath.Join(root, "gradle.properties")
	if err := os.Chmod(pp, 0o444); err != nil {
		t.Fatal(err)
	}
	upd := newFileUpdates()
	upd.props["libVersion"] = "9.9.9"
	if err := applyUpdates(root, "", upd); err == nil {
		_ = os.Chmod(root, 0o555)
		defer func() { _ = os.Chmod(root, 0o755) }()
		if err := applyUpdates(root, "", upd); err == nil {
			t.Fatal("expected properties update error")
		}
	}
	_ = os.Chmod(pp, 0o644)
	_ = os.Chmod(root, 0o755)

	bp := filepath.Join(root, "build.gradle")
	_ = os.Chmod(root, 0o755)
	if err := os.Chmod(bp, 0o444); err != nil {
		t.Fatal(err)
	}
	_ = os.Chmod(root, 0o555)
	defer func() {
		_ = os.Chmod(root, 0o755)
		_ = os.Chmod(bp, 0o644)
	}()
	upd2 := newFileUpdates()
	upd2.pluginsByFile[bp] = map[string]string{"com.ex.plugin": "9.9.9"}
	if err := applyUpdates(root, "", upd2); err == nil {
		t.Fatal("expected plugin update error")
	}
	_ = os.Chmod(root, 0o755)
	_ = os.Chmod(bp, 0o644)

	wp := gradle.WrapperPropertiesPath(root)
	if err := os.Chmod(wp, 0o444); err != nil {
		t.Fatal(err)
	}
	_ = os.Chmod(filepath.Dir(wp), 0o555)
	defer func() {
		_ = os.Chmod(filepath.Dir(wp), 0o755)
		_ = os.Chmod(wp, 0o644)
	}()
	upd3 := newFileUpdates()
	upd3.wrapper = "9.9.9"
	if err := applyUpdates(root, "", upd3); err == nil {
		t.Fatal("expected wrapper update error")
	}
}

func TestRunApplyUpdatesError(t *testing.T) {
	srv := startRepo(t)
	defer srv.Close()
	root := t.TempDir()
	writeProject(t, root)
	wpath := gradle.WrapperPropertiesPath(root)
	if err := os.WriteFile(wpath, []byte(
		"distributionUrl="+strings.ReplaceAll(srv.URL+"/dist/gradle-8.0-bin.zip", ":", `\:`)+"\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}
	// freeze gradle.properties against writes after dry resolve would want updates
	if err := os.Chmod(filepath.Join(root, "gradle.properties"), 0o444); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o555); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chmod(root, 0o755)
		_ = os.Chmod(filepath.Join(root, "gradle.properties"), 0o644)
	}()
	_, err := Run(context.Background(), Options{
		Root:  root,
		Repos: []string{srv.URL + "/repo"},
		Apply: true,
	})
	if err == nil {
		t.Fatal("expected apply error")
	}
}
