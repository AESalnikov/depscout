package maven

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AESalnikov/depscout/internal/gradle"
)

func TestArtifactoryLatestVersionURL(t *testing.T) {
	u, ok := ArtifactoryLatestVersionURL("https://h/artifactory/public", "com.ex", "lib")
	if !ok || !strings.Contains(u, "/api/search/latestVersion?") || !strings.Contains(u, "repos=public") || !strings.Contains(u, "g=com.ex") {
		t.Fatal(u, ok)
	}
	u, ok = ArtifactoryLatestVersionURL("https://h/artifactory/list/public/", "a.b", "c")
	if !ok || !strings.Contains(u, "repos=public") {
		t.Fatal(u, ok)
	}
	if _, ok := ArtifactoryLatestVersionURL("https://repo1.maven.org/maven2", "a", "b"); ok {
		t.Fatal("central")
	}
	if _, ok := ArtifactoryLatestVersionURL("https://h/artifactory/a/b", "x", "y"); ok {
		t.Fatal("nested repo key")
	}
	if VersionFromArtifactoryLatestVersionBody([]byte("  10.1.3\n")) != "10.1.3" {
		t.Fatal("plain")
	}
	if VersionFromArtifactoryLatestVersionBody([]byte(`{"x":1}`)) != "" || VersionFromArtifactoryLatestVersionBody([]byte("<html>")) != "" {
		t.Fatal("reject")
	}
}

func TestVersionsFromArtifactoryStorageJSON(t *testing.T) {
	body := []byte(`{"children":[
		{"uri":"/10.0.6"},{"uri":"/10.1.3"},{"uri":"/10.1.3"},
		{"uri":"/10.1.3-SNAPSHOT"},{"uri":"/maven-metadata.xml"},
		{"uri":"/nodigits"},{"uri":"/nested/path"},{"uri":"/"}
	]}`)
	if PickLatest(VersionsFromArtifactoryStorageJSON(body), false) != "10.1.3" {
		t.Fatal()
	}
	if VersionsFromArtifactoryStorageJSON([]byte("not json")) != nil ||
		VersionsFromArtifactoryStorageJSON([]byte(`[]`)) != nil ||
		VersionsFromArtifactoryStorageJSON(nil) != nil {
		t.Fatal()
	}
	padded := []byte("\r\n\t  {\"children\":[{\"uri\":\"/1.0\"}]}")
	if PickLatest(VersionsFromArtifactoryStorageJSON(padded), false) != "1.0" {
		t.Fatal("trim")
	}
}

func TestResolveViaArtifactoryLatestVersionAPI(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/artifactory/public/com/ex/plug/com.ex.plug.gradle.plugin/maven-metadata.xml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<metadata><versioning><versions><version>1.0.0-SNAPSHOT</version></versions></versioning></metadata>`))
	})
	mux.HandleFunc("/artifactory/public/com/ex/plug/com.ex.plug.gradle.plugin/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "maven-metadata") {
			return
		}
		_, _ = w.Write([]byte(`<!DOCTYPE html><html><body>Index of (empty)</body></html>`))
	})
	mux.HandleFunc("/artifactory/api/search/latestVersion", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("10.1.3\n"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := &Client{HTTP: srv.Client(), Repos: []string{srv.URL + "/artifactory/public"}}
	res := c.ResolveLatest(context.Background(), gradle.GAV{
		Group: "com.ex.plug", Artifact: "com.ex.plug.gradle.plugin",
	})
	if !res.Found || res.Latest != "10.1.3" {
		t.Fatalf("%+v", res)
	}
}

func TestResolveLatestVersionIgnoredPreRelease(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/artifactory/public/g/a/maven-metadata.xml", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/artifactory/public/g/a/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "maven-metadata") {
			return
		}
		_, _ = w.Write([]byte(`<html></html>`))
	})
	mux.HandleFunc("/artifactory/api/search/latestVersion", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("1.0.0-SNAPSHOT"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := &Client{HTTP: srv.Client(), Repos: []string{srv.URL + "/artifactory/public"}}
	res := c.ResolveLatest(context.Background(), gradle.GAV{Group: "g", Artifact: "a"})
	if res.Found {
		t.Fatalf("%+v", res)
	}
}

func TestArtifactoryAPIErrorPaths(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/artifactory/public/g/a/maven-metadata.xml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<metadata><versioning><versions><version>1.0.0-SNAPSHOT</version></versions></versioning></metadata>`))
	})
	mux.HandleFunc("/artifactory/public/g/a/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "maven-metadata") {
			return
		}
		_, _ = w.Write([]byte(`<html></html>`))
	})
	mux.HandleFunc("/artifactory/api/search/latestVersion", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := &Client{HTTP: srv.Client(), Repos: []string{srv.URL + "/artifactory/public"}}
	res := c.ResolveLatest(context.Background(), gradle.GAV{Group: "g", Artifact: "a"})
	if res.Found {
		t.Fatalf("%+v", res)
	}

	mux2 := http.NewServeMux()
	mux2.HandleFunc("/artifactory/public/g/a/maven-metadata.xml", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux2.HandleFunc("/artifactory/public/g/a/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "maven-metadata") {
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	mux2.HandleFunc("/artifactory/api/search/latestVersion", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	srv2 := httptest.NewServer(mux2)
	defer srv2.Close()
	c2 := &Client{HTTP: srv2.Client(), Repos: []string{srv2.URL + "/artifactory/public"}}
	res = c2.ResolveLatest(context.Background(), gradle.GAV{Group: "g", Artifact: "a"})
	if res.Found || res.Err != nil {
		t.Fatalf("all 404 → not found: %+v", res)
	}

	closed := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	closed.Close()
	c3 := &Client{HTTP: closed.Client(), Repos: []string{closed.URL + "/artifactory/public"}}
	_, err := c3.versionsFromArtifactoryAPI(context.Background(), closed.URL+"/artifactory/public", gradle.GAV{Group: "g", Artifact: "a"})
	if err == nil {
		t.Fatal("want net err")
	}
	vers, err := c3.versionsFromArtifactoryAPI(context.Background(), "https://repo1.maven.org/maven2", gradle.GAV{Group: "g", Artifact: "a"})
	if err != nil || vers != nil {
		t.Fatal(vers, err)
	}
}

func TestVersionsFromListingJSONBody(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/artifactory/public/g/a/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"children":[{"uri":"/2.0.0"},{"uri":"/1.0.0"}]}`))
	})
	mux.HandleFunc("/artifactory/public/g/a/maven-metadata.xml", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := &Client{HTTP: srv.Client(), Repos: []string{srv.URL + "/artifactory/public"}}
	res := c.ResolveLatest(context.Background(), gradle.GAV{Group: "g", Artifact: "a"})
	if !res.Found || res.Latest != "2.0.0" {
		t.Fatalf("%+v", res)
	}
}

func TestBareHTMLIndexVersions(t *testing.T) {
	html := "Index of public/.../plugin\n10.0.6/    01-Jan-2026\n10.1.3/    30-Sep-2026\n"
	if PickLatest(VersionsFromHTMLListing(html), false) != "10.1.3" {
		t.Fatal()
	}
}

func TestResolveViaHTMLIndexDespiteJSONPrefer(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/artifactory/public/g/a/maven-metadata.xml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<metadata><versioning><versions><version>1.0.0-SNAPSHOT</version></versions></versioning></metadata>`))
	})
	mux.HandleFunc("/artifactory/public/g/a/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "maven-metadata") {
			return
		}
		accept := r.Header.Get("Accept")
		if strings.Contains(accept, "application/json") && !strings.Contains(accept, "text/html") {
			_, _ = w.Write([]byte(`{"repo":"public","path":"/g/a","folder":true}`))
			return
		}
		_, _ = w.Write([]byte(`Index of /g/a
<a href="10.0.6/">10.0.6/</a>
<a href="10.1.3/">10.1.3/</a>
`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := &Client{HTTP: srv.Client(), Repos: []string{srv.URL + "/artifactory/public"}}
	res := c.ResolveLatest(context.Background(), gradle.GAV{Group: "g", Artifact: "a"})
	if !res.Found || res.Latest != "10.1.3" {
		t.Fatalf("%+v", res)
	}
}
