package wrapper

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AESalnikov/depscout/internal/maven"
)

func TestLatestGradle(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/dist/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`
<a href="gradle-9.6.1-bin.zip">gradle-9.6.1-bin.zip</a>
<a href="gradle-9.8.0-bin.zip">gradle-9.8.0-bin.zip</a>
<a href="gradle-9.8.0-all.zip">gradle-9.8.0-all.zip</a>
<a href="gradle-9.9.0-rc-1-bin.zip">gradle-9.9.0-rc-1-bin.zip</a>
<a href="gradle-9.8.0-bin.zip">dup</a>
`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := &maven.Client{HTTP: srv.Client()}
	r := &Resolver{Client: c, IncludePreRelease: false}
	got, err := r.LatestGradle(context.Background(), srv.URL+"/dist", "bin")
	if err != nil || got != "9.8.0" {
		t.Fatalf("%s %v", got, err)
	}

	// all classifier
	got, err = r.LatestGradle(context.Background(), srv.URL+"/dist/", "all")
	if err != nil || got != "9.8.0" {
		t.Fatalf("all %s %v", got, err)
	}
}

func TestLatestGradleCorners(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/empty/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`no zips`))
	})
	mux.HandleFunc("/err/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/all-only/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<a href="gradle-9.9.0-all.zip">x</a>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := &maven.Client{HTTP: srv.Client()}
	r := &Resolver{Client: c}

	if _, err := r.LatestGradle(context.Background(), srv.URL+"/empty", "bin"); err == nil || !strings.Contains(err.Error(), "no gradle") {
		t.Fatalf("%v", err)
	}
	if _, err := r.LatestGradle(context.Background(), srv.URL+"/err", "bin"); err == nil || !strings.Contains(err.Error(), "HTTP") {
		t.Fatalf("%v", err)
	}
	// classifier filter leaves nothing
	if _, err := r.LatestGradle(context.Background(), srv.URL+"/all-only", "bin"); err == nil {
		t.Fatal("expected no bin distributions")
	}
	r.IncludePreRelease = true
	got, err := r.LatestGradle(context.Background(), srv.URL+"/all-only", "all")
	if err != nil || got != "9.9.0" {
		t.Fatalf("%s %v", got, err)
	}

	closed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	closed.Close()
	r2 := &Resolver{Client: &maven.Client{HTTP: closed.Client()}}
	if _, err := r2.LatestGradle(context.Background(), closed.URL+"/d", "bin"); err == nil {
		t.Fatal("net error expected")
	}
}
