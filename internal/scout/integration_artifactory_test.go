//go:build integration

package scout_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/AESalnikov/depscout/internal/report"
	"github.com/AESalnikov/depscout/internal/scout"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func artifactoryFixtureDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("Caller")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "artifactory-tc")
}

// TestIntegrationArtifactoryTestcontainers поднимает nginx-фикстуру Artifactory layout в Docker
// и гоняет scout.Run против реального HTTP (не httptest).
//
// Запуск: make integration  (нужен Docker)
func TestIntegrationArtifactoryTestcontainers(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	fixture := artifactoryFixtureDir(t)
	ctr, err := testcontainers.Run(
		ctx,
		"",
		testcontainers.WithDockerfile(testcontainers.FromDockerfile{
			Context:    fixture,
			Dockerfile: "Dockerfile",
			Repo:       "depscout-artifactory-tc",
			Tag:        "test",
		}),
		testcontainers.WithExposedPorts("80/tcp"),
		testcontainers.WithWaitStrategy(
			wait.ForHTTP("/healthz").WithPort("80/tcp").WithStartupTimeout(60*time.Second),
		),
	)
	testcontainers.CleanupContainer(t, ctr)
	if err != nil {
		t.Fatalf("start container: %v", err)
	}

	base, err := ctr.PortEndpoint(ctx, "80/tcp", "http")
	if err != nil {
		t.Fatal(err)
	}
	repoURL := base + "/artifactory/public"

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "build.gradle"), []byte(`
plugins {
    id "org.springframework.boot" version "3.3.0"
}
dependencies {
    implementation "com.google.guava:guava:${guavaVersion}"
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "gradle.properties"), []byte("guavaVersion=33.0.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := scout.Run(ctx, scout.Options{
		Root:        root,
		Repos:       []string{repoURL},
		SkipCatalog: true,
		SkipWrapper: true,
		SkipPom:     true,
	})
	if err != nil {
		t.Fatal(err)
	}

	var plugin, prop *report.Row
	for i := range res.Rows {
		r := &res.Rows[i]
		switch r.Kind {
		case report.KindPlugin:
			plugin = r
		case report.KindProperty:
			prop = r
		}
	}
	if plugin == nil || plugin.Status != report.StatusOutdated || plugin.Latest != "3.4.1" {
		t.Fatalf("plugin via marker-impl: %+v rows=%+v", plugin, res.Rows)
	}
	if prop == nil || prop.Status != report.StatusOutdated || prop.Latest != "33.4.0" {
		t.Fatalf("property via latestVersion: %+v rows=%+v", prop, res.Rows)
	}
}
