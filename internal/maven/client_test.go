package maven

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AESalnikov/depscout/internal/gradle"
)

func TestAuthFromEnvAndNewClient(t *testing.T) {
	t.Setenv("DEPSCOUT_USER", "")
	t.Setenv("DEPSCOUT_PASSWORD", "")
	t.Setenv("ARTIFACTORY_USER", "au")
	t.Setenv("ARTIFACTORY_PASSWORD", "ap")
	u, p := AuthFromEnv()
	if u != "au" || p != "ap" {
		t.Fatalf("%s %s", u, p)
	}
	t.Setenv("DEPSCOUT_USER", "du")
	t.Setenv("DEPSCOUT_PASSWORD", "dp")
	u, p = AuthFromEnv()
	if u != "du" || p != "dp" {
		t.Fatalf("%s %s", u, p)
	}
	c := NewClient([]string{"https://example.com/r"}, false)
	if c.User != "du" || c.Password != "dp" || c.Concurrency != 8 {
		t.Fatalf("%+v", c)
	}
	if firstNonEmpty("", "", "x") != "x" || firstNonEmpty() != "" {
		t.Fatal("firstNonEmpty")
	}
}

func TestResolveLatestMetadataAndListing(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repo/com/ex/lib/maven-metadata.xml", func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "u" || pass != "p" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<metadata><versioning>
  <latest>1.2.0-SNAPSHOT</latest>
  <release>1.1.0</release>
  <versions><version>1.0.0</version><version>1.1.0</version><version>1.2.0-SNAPSHOT</version></versions>
</versioning></metadata>`))
	})
	mux.HandleFunc("/repo/com/ex/lib/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "maven-metadata.xml") {
			return
		}
		_, _ = w.Write([]byte(`<a href="1.1.0/">1.1.0/</a><a href="1.3.0/">1.3.0/</a>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := &Client{
		HTTP:        srv.Client(),
		Repos:       []string{srv.URL + "/repo"},
		User:        "u",
		Password:    "p",
		Concurrency: 2,
	}
	res := c.ResolveLatest(context.Background(), gradle.GAV{Group: "com.ex", Artifact: "lib"})
	if !res.Found || res.Latest != "1.3.0" {
		t.Fatalf("%+v", res)
	}

	many := c.ResolveMany(context.Background(), []gradle.GAV{
		{Group: "com.ex", Artifact: "lib"},
		{Group: "com.ex", Artifact: "missing"},
	})
	if !many[0].Found || many[1].Found {
		t.Fatalf("%+v", many)
	}
}

func TestResolveLatestCorners(t *testing.T) {
	mux := http.NewServeMux()
	// first repo: bad metadata HTML 200, listing empty → error path
	mux.HandleFunc("/bad/com/ex/lib/maven-metadata.xml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<!DOCTYPE html><html></html>"))
	})
	mux.HandleFunc("/bad/com/ex/lib/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "maven-metadata") {
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	// second repo: 404 metadata, good listing
	mux.HandleFunc("/ok/com/ex/lib/maven-metadata.xml", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/ok/com/ex/lib/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "maven-metadata") {
			return
		}
		_, _ = w.Write([]byte(`<a href="2.0.0/">2.0.0/</a>`))
	})
	// auth failure then listing works on same repo
	mux.HandleFunc("/auth/com/ex/lib/maven-metadata.xml", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	mux.HandleFunc("/auth/com/ex/lib/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "maven-metadata") {
			return
		}
		_, _ = w.Write([]byte(`<a href="3.0.0/">3.0.0/</a>`))
	})
	// HTTP 500
	mux.HandleFunc("/err/com/ex/lib/maven-metadata.xml", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	mux.HandleFunc("/err/com/ex/lib/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "maven-metadata") {
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	})
	// empty versions
	mux.HandleFunc("/empty/com/ex/lib/maven-metadata.xml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<metadata><versioning></versioning></metadata>`))
	})
	mux.HandleFunc("/empty/com/ex/lib/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "maven-metadata") {
			return
		}
		_, _ = w.Write([]byte(`no versions here`))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	gav := gradle.GAV{Group: "com.ex", Artifact: "lib"}

	c := &Client{HTTP: srv.Client(), Repos: []string{srv.URL + "/bad", srv.URL + "/ok"}, Concurrency: 0}
	res := c.ResolveLatest(context.Background(), gav)
	if !res.Found || res.Latest != "2.0.0" {
		t.Fatalf("fallback repo: %+v", res)
	}

	c2 := &Client{HTTP: srv.Client(), Repos: []string{srv.URL + "/auth"}}
	res = c2.ResolveLatest(context.Background(), gav)
	if !res.Found || res.Latest != "3.0.0" {
		t.Fatalf("auth+listing: %+v", res)
	}

	c3 := &Client{HTTP: srv.Client(), Repos: []string{srv.URL + "/err"}}
	res = c3.ResolveLatest(context.Background(), gav)
	if res.Found || res.Err == nil {
		t.Fatalf("want error: %+v", res)
	}

	c4 := &Client{HTTP: srv.Client(), Repos: []string{srv.URL + "/empty"}}
	res = c4.ResolveLatest(context.Background(), gav)
	if res.Found {
		t.Fatalf("want not found: %+v", res)
	}

	// пустой ответ на первом репо + ошибка на втором → NOT_FOUND (не ERROR)
	c4b := &Client{HTTP: srv.Client(), Repos: []string{srv.URL + "/empty", srv.URL + "/err"}}
	res = c4b.ResolveLatest(context.Background(), gav)
	if res.Found || res.Err != nil {
		t.Fatalf("empty then err should be not-found: %+v", res)
	}

	c5 := &Client{HTTP: srv.Client(), Repos: nil}
	res = c5.ResolveLatest(context.Background(), gav)
	if res.Found || res.Err == nil || !strings.Contains(res.Err.Error(), "no repositories") {
		t.Fatalf("%+v", res)
	}

	// network error to closed server
	closed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	closed.Close()
	c6 := &Client{HTTP: closed.Client(), Repos: []string{closed.URL + "/repo"}}
	res = c6.ResolveLatest(context.Background(), gav)
	if res.Found || res.Err == nil {
		t.Fatalf("want net error: %+v", res)
	}

	body, status, err := c.GetBytes(context.Background(), srv.URL+"/ok/com/ex/lib/")
	if err != nil || status != 200 || len(body) == 0 {
		t.Fatalf("GetBytes %d %v %q", status, err, body)
	}
}

func TestVersionsFromListingHTTPStatus(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/r/g/a/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), Repos: []string{srv.URL + "/r"}}
	_, err := c.versionsFromListing(context.Background(), srv.URL+"/r", gradle.GAV{Group: "g", Artifact: "a"})
	if err == nil || !strings.Contains(err.Error(), "418") {
		t.Fatalf("%v", err)
	}
}

func TestGetInvalidURL(t *testing.T) {
	c := &Client{HTTP: http.DefaultClient}
	_, _, err := c.get(context.Background(), "://bad-url", acceptHTML)
	if err == nil {
		t.Fatal("expected error")
	}
}

type errReadCloser struct{}

func (errReadCloser) Read([]byte) (int, error) { return 0, fmt.Errorf("read fail") }
func (errReadCloser) Close() error             { return nil }

func TestGetReadBodyError(t *testing.T) {
	c := &Client{HTTP: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Body:       errReadCloser{},
		}, nil
	})}}
	_, _, err := c.get(context.Background(), "http://example.com/x", acceptHTML)
	if err == nil || !strings.Contains(err.Error(), "read fail") {
		t.Fatalf("%v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestResolveListErrWhenMetaMissing(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/r/g/a/maven-metadata.xml", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/r/g/a/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "maven-metadata") {
			return
		}
		w.WriteHeader(http.StatusBadGateway)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), Repos: []string{srv.URL + "/r"}}
	res := c.ResolveLatest(context.Background(), gradle.GAV{Group: "g", Artifact: "a"})
	if res.Found || res.Err == nil || !strings.Contains(res.Err.Error(), "502") {
		t.Fatalf("%+v", res)
	}
}

func TestResolveManyDefaultConcurrency(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), Repos: []string{srv.URL}, Concurrency: -1}
	out := c.ResolveMany(context.Background(), []gradle.GAV{{Group: "g", Artifact: "a"}})
	if len(out) != 1 || out[0].Found {
		t.Fatalf("%+v", out)
	}
}

func TestResolveInRepoMetaErrButListingOK(t *testing.T) {
	// meta returns 401, listing 404 → metaErr returned
	mux := http.NewServeMux()
	mux.HandleFunc("/r/g/a/maven-metadata.xml", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	mux.HandleFunc("/r/g/a/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "maven-metadata") {
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), Repos: []string{srv.URL + "/r"}}
	res := c.ResolveLatest(context.Background(), gradle.GAV{Group: "g", Artifact: "a"})
	if res.Found || res.Err == nil || !strings.Contains(res.Err.Error(), "401") {
		t.Fatalf("%+v", res)
	}
	_ = fmt.Sprintf("%v", res)
}
