package maven

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AESalnikov/depscout/internal/gradle"
)

func TestVersionFromArtifactoryLatestVersionBodyNewline(t *testing.T) {
	if VersionFromArtifactoryLatestVersionBody([]byte("10.1.3\ntrailing")) != "10.1.3" {
		t.Fatal("newline cut")
	}
	if VersionFromArtifactoryLatestVersionBody([]byte("")) != "" {
		t.Fatal("empty")
	}
}

func TestVersionsFromArtifactoryStorageJSONBadJSON(t *testing.T) {
	if VersionsFromArtifactoryStorageJSON([]byte(`{not-json`)) != nil {
		t.Fatal("want nil on unmarshal error")
	}
}

func TestNewClientDebugEnvAndDebugf(t *testing.T) {
	t.Setenv("DEPSCOUT_DEBUG", "1")
	t.Setenv("DEPSCOUT_USER", "")
	t.Setenv("DEPSCOUT_PASSWORD", "")
	t.Setenv("ARTIFACTORY_USER", "")
	t.Setenv("ARTIFACTORY_PASSWORD", "")
	c := NewClient([]string{"https://example.com/r"}, false)
	if c.Debug == nil {
		t.Fatal("want Debug from DEPSCOUT_DEBUG")
	}
	var buf bytes.Buffer
	c.Debug = &buf
	c.debugf("hello %s", "world")
	if !strings.Contains(buf.String(), "depscout: hello world") {
		t.Fatalf("%q", buf.String())
	}
	(&Client{}).debugf("noop") // Debug nil
}

func TestVersionsFromArtifactoryAPIEmptyBody(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/artifactory/api/search/latestVersion", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"errors":[]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := &Client{HTTP: srv.Client()}
	vers, err := c.versionsFromArtifactoryAPI(context.Background(), srv.URL+"/artifactory/public", gradle.GAV{Group: "g", Artifact: "a"})
	if err != nil || vers != nil {
		t.Fatal(vers, err)
	}
}

func TestImplementationGAVsSkipMarkerAndDedupe(t *testing.T) {
	data := []byte(`<?xml version="1.0"?><project>
  <dependencies>
    <dependency><groupId>com.ex</groupId><artifactId>com.ex.gradle.plugin</artifactId></dependency>
    <dependency><groupId>com.ex</groupId><artifactId>impl</artifactId></dependency>
    <dependency><groupId>com.ex</groupId><artifactId>impl</artifactId></dependency>
    <dependency><groupId></groupId><artifactId>x</artifactId></dependency>
  </dependencies>
</project>`)
	gavs := ImplementationGAVsFromMarkerPOM(data)
	if len(gavs) != 1 || gavs[0].Artifact != "impl" {
		t.Fatalf("%v", gavs)
	}
}

func TestResolveViaMarkerImplementationMissPaths(t *testing.T) {
	// marker POM 404 → no impls
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), Repos: []string{srv.URL + "/artifactory/public"}}
	marker := gradle.GAV{Group: "com.ex.p", Artifact: "com.ex.p.gradle.plugin"}
	if _, ok := c.resolveViaMarkerImplementation(context.Background(), marker, "1.0.0"); ok {
		t.Fatal("want miss on 404 pom")
	}

	// marker POM ok, impl not found, or impl found but marker POM for latest missing
	mux2 := http.NewServeMux()
	mux2.HandleFunc("/artifactory/public/com/ex/p/com.ex.p.gradle.plugin/1.0.0/com.ex.p.gradle.plugin-1.0.0.pom", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<?xml version="1.0"?><project><dependencies>
  <dependency><groupId>com.ex</groupId><artifactId>impl</artifactId></dependency>
</dependencies></project>`))
	})
	mux2.HandleFunc("/artifactory/public/com/ex/impl/maven-metadata.xml", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux2.HandleFunc("/artifactory/public/com/ex/impl/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "maven-metadata") {
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	srv2 := httptest.NewServer(mux2)
	defer srv2.Close()
	c2 := &Client{HTTP: srv2.Client(), Repos: []string{srv2.URL + "/artifactory/public"}}
	if _, ok := c2.resolveViaMarkerImplementation(context.Background(), marker, "1.0.0"); ok {
		t.Fatal("want miss when impl not found")
	}

	// impl found via latestVersion, but marker POM for that version missing → miss
	mux3 := http.NewServeMux()
	mux3.HandleFunc("/artifactory/public/com/ex/p/com.ex.p.gradle.plugin/1.0.0/com.ex.p.gradle.plugin-1.0.0.pom", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<?xml version="1.0"?><project><dependencies>
  <dependency><groupId>com.ex</groupId><artifactId>impl</artifactId></dependency>
</dependencies></project>`))
	})
	mux3.HandleFunc("/artifactory/public/com/ex/impl/maven-metadata.xml", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux3.HandleFunc("/artifactory/public/com/ex/impl/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "maven-metadata") {
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	mux3.HandleFunc("/artifactory/api/search/latestVersion", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("a") == "impl" {
			_, _ = w.Write([]byte("2.0.0"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	// marker POM for 2.0.0 absent
	mux3.HandleFunc("/artifactory/public/com/ex/p/com.ex.p.gradle.plugin/2.0.0/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	srv3 := httptest.NewServer(mux3)
	defer srv3.Close()
	c3 := &Client{HTTP: srv3.Client(), Repos: []string{srv3.URL + "/artifactory/public"}}
	res := c3.ResolveLatestWithCurrent(context.Background(), marker, "1.0.0")
	if res.Found {
		t.Fatalf("want marker-impl miss: %+v", res)
	}
}
