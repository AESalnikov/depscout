package maven

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AESalnikov/depscout/internal/gradle"
)

func TestImplementationGAVsFromMarkerPOM(t *testing.T) {
	data := []byte(`<?xml version="1.0"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
  <dependencies>
    <dependency>
      <groupId>org.springframework.boot</groupId>
      <artifactId>spring-boot-gradle-plugin</artifactId>
      <version>3.4.1</version>
    </dependency>
  </dependencies>
</project>`)
	gavs := ImplementationGAVsFromMarkerPOM(data)
	if len(gavs) != 1 || gavs[0].Group != "org.springframework.boot" || gavs[0].Artifact != "spring-boot-gradle-plugin" {
		t.Fatalf("%v", gavs)
	}
	if ImplementationGAVsFromMarkerPOM(nil) != nil || ImplementationGAVsFromMarkerPOM([]byte("<!DOCTYPE html>")) != nil {
		t.Fatal("bad")
	}
	if ImplementationGAVsFromMarkerPOM([]byte(`{not xml`)) != nil {
		t.Fatal("broken")
	}
	if !IsGradlePluginMarker("x.gradle.plugin") || IsGradlePluginMarker("x") {
		t.Fatal("marker")
	}
	u := ArtifactPOMURL("https://h/r/", "a.b", "c", "1.0")
	if u != "https://h/r/a/b/c/1.0/c-1.0.pom" {
		t.Fatal(u)
	}
}

func TestResolveLatestWithCurrentViaImpl(t *testing.T) {
	mux := http.NewServeMux()
	// marker: только SNAPSHOT в metadata, пустой Index, latestVersion 404
	mux.HandleFunc("/artifactory/public/org/springframework/boot/org.springframework.boot.gradle.plugin/maven-metadata.xml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<metadata><versioning><versions><version>3.3.0-SNAPSHOT</version></versions></versioning></metadata>`))
	})
	mux.HandleFunc("/artifactory/public/org/springframework/boot/org.springframework.boot.gradle.plugin/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "maven-metadata") {
			return
		}
		if strings.HasSuffix(r.URL.Path, ".pom") {
			ver := ""
			for _, v := range []string{"3.3.0", "3.4.1"} {
				if strings.Contains(r.URL.Path, "/"+v+"/") {
					ver = v
					break
				}
			}
			if ver == "" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write([]byte(`<?xml version="1.0"?><project xmlns="http://maven.apache.org/POM/4.0.0">
  <dependencies><dependency>
    <groupId>org.springframework.boot</groupId><artifactId>spring-boot-gradle-plugin</artifactId><version>` + ver + `</version>
  </dependency></dependencies></project>`))
			return
		}
		_, _ = w.Write([]byte(`<html></html>`))
	})
	mux.HandleFunc("/artifactory/api/search/latestVersion", func(w http.ResponseWriter, r *http.Request) {
		a := r.URL.Query().Get("a")
		if a == "spring-boot-gradle-plugin" {
			_, _ = w.Write([]byte("3.4.1"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/artifactory/public/org/springframework/boot/spring-boot-gradle-plugin/maven-metadata.xml", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/artifactory/public/org/springframework/boot/spring-boot-gradle-plugin/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "maven-metadata") {
			return
		}
		_, _ = w.Write([]byte(`<html></html>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := &Client{HTTP: srv.Client(), Repos: []string{srv.URL + "/artifactory/public"}}
	marker := gradle.GAV{
		Group:    "org.springframework.boot",
		Artifact: "org.springframework.boot.gradle.plugin",
	}
	res := c.ResolveLatestWithCurrent(context.Background(), marker, "3.3.0")
	if !res.Found || res.Latest != "3.4.1" {
		t.Fatalf("%+v", res)
	}

	res = c.ResolveLatestWithCurrent(context.Background(), marker, "")
	if res.Found {
		t.Fatalf("want not found: %+v", res)
	}

	results := c.ResolveManyWithCurrent(context.Background(), []gradle.GAV{marker}, []string{"3.3.0"})
	if len(results) != 1 || !results[0].Found || results[0].Latest != "3.4.1" {
		t.Fatalf("%+v", results)
	}
}
