// Package scout оркестрирует один запуск depscout: парсеры, resolve версий и apply.
// Печать отчёта остаётся в main — пакет возвращает Result.
package scout

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/AESalnikov/depscout/internal/gradle"
	"github.com/AESalnikov/depscout/internal/maven"
	"github.com/AESalnikov/depscout/internal/pom"
	"github.com/AESalnikov/depscout/internal/report"
)

// absPath — обёртка над filepath.Abs; в тестах подменяем, чтобы проверить ошибку пути.
var absPath = filepath.Abs

// Options — параметры одного запуска.
type Options struct {
	Root              string            // корень Gradle- и/или Maven-проекта
	Apply             bool              // true → записать обновления на диск
	IncludePreRelease bool              // учитывать SNAPSHOT/feature
	Repos             []string          // явные Maven base URL
	HostRewrites      map[string]string // old.host → new.host (часто для wrapper URL)
	MapFile           string            // опциональный файл prop=group:artifact
	CatalogFile       string            // путь к libs.versions.toml; пусто → gradle/libs.versions.toml
	SkipDeps          bool              // gradle.properties + inline в build-скриптах
	SkipCatalog       bool              // Version Catalog
	SkipPlugins       bool              // блок plugins в Gradle
	SkipWrapper       bool              // Gradle Wrapper
	SkipPom           bool              // Maven pom.xml
	Debug             bool              // подробный лог resolve/HTTP на stderr
}

// Result — итог прогона: строки отчёта и фактически использованные repos.
type Result struct {
	Rows  []report.Row
	Repos []string
}

// fileUpdates — накопленные правки для --apply по разным источникам.
type fileUpdates struct {
	props           map[string]string            // gradle.properties
	pluginsByFile   map[string]map[string]string // build-скрипт → pluginId → ver
	catalogVersions map[string]string            // [versions] keys
	catalogLiterals map[string]string            // library/plugin alias → ver
	inlineByFile    map[string]map[string]string // build-скрипт → g:a → ver
	pomProps        map[string]string            // pom.xml <properties>
	pomLiterals     map[string]string            // pom.xml g:a → ver
	wrapper         string
}

func newFileUpdates() *fileUpdates {
	return &fileUpdates{
		props:           map[string]string{},
		pluginsByFile:   map[string]map[string]string{},
		catalogVersions: map[string]string{},
		catalogLiterals: map[string]string{},
		inlineByFile:    map[string]map[string]string{},
		pomProps:        map[string]string{},
		pomLiterals:     map[string]string{},
	}
}

// Run выполняет полный цикл: parse → resolve → (опционально) apply.
// ctx используется для отмены HTTP при Ctrl+C.
func Run(ctx context.Context, opt Options) (*Result, error) {
	root, err := absPath(opt.Root)
	if err != nil {
		return nil, err
	}
	if err := ensureProject(root); err != nil {
		return nil, err
	}

	rewrites := opt.HostRewrites
	if rewrites == nil {
		rewrites = map[string]string{}
	}
	envRewrites, err := maven.HostRewritesFromEnv()
	if err != nil {
		return nil, err
	}
	rewrites = maven.MergeRewrites(envRewrites, rewrites)

	repos := maven.RewriteURLs(opt.Repos, rewrites)
	if len(repos) == 0 && needsMavenRepos(root, opt) {
		return nil, fmt.Errorf("no Maven repositories configured; pass --repos or set DEPSCOUT_REPOS")
	}

	client := maven.NewClient(repos, opt.IncludePreRelease)
	if opt.Debug {
		client.Debug = os.Stderr
	}
	var rows []report.Row
	upd := newFileUpdates()

	// Приоритет при дедупе GAV: catalog > pom > property > inline.
	claimed := map[string]bool{}
	isGradle := gradle.IsGradleProject(root)

	if isGradle && !opt.SkipCatalog {
		catRows, err := scoutCatalog(ctx, root, client, opt.CatalogFile, claimed, upd)
		if err != nil {
			return nil, err
		}
		rows = append(rows, catRows...)
	}

	if !opt.SkipPom && pom.Exists(root) {
		pomRows, err := scoutPom(ctx, root, client, claimed, upd)
		if err != nil {
			return nil, err
		}
		rows = append(rows, pomRows...)
	}

	if isGradle && !opt.SkipDeps {
		depRows, err := scoutDeps(ctx, root, client, opt.MapFile, claimed, upd)
		if err != nil {
			return nil, err
		}
		rows = append(rows, depRows...)

		inlineRows, err := scoutInline(ctx, root, client, claimed, upd)
		if err != nil {
			return nil, err
		}
		rows = append(rows, inlineRows...)
	}

	if isGradle && !opt.SkipPlugins {
		pluginRows, err := scoutPlugins(ctx, root, client, upd)
		if err != nil {
			return nil, err
		}
		rows = append(rows, pluginRows...)
	}

	if isGradle && !opt.SkipWrapper {
		if _, err := os.Stat(gradle.WrapperPropertiesPath(root)); err == nil {
			wRow, newVer, err := scoutWrapper(ctx, root, client, opt.IncludePreRelease, rewrites)
			if err != nil {
				rows = append(rows, report.Row{
					Kind:   report.KindWrapper,
					Name:   "gradle",
					Source: maven.ShortError(err),
					Status: report.StatusError,
				})
			} else {
				rows = append(rows, wRow)
				if wRow.Status == report.StatusOutdated {
					upd.wrapper = newVer
				}
			}
		}
	}

	if opt.Apply {
		if err := applyUpdates(root, opt.CatalogFile, upd); err != nil {
			return nil, err
		}
	}

	return &Result{Rows: rows, Repos: repos}, nil
}

// ensureProject проверяет build.gradle / build.gradle.kts или pom.xml.
func ensureProject(root string) error {
	if gradle.IsGradleProject(root) || pom.Exists(root) {
		return nil
	}
	return fmt.Errorf("%s: not a Gradle/Maven project (need build.gradle, build.gradle.kts or pom.xml)", root)
}

func catalogPath(root, override string) string {
	if override != "" {
		return override
	}
	return gradle.DefaultCatalogPath(root)
}

// needsMavenRepos — нужны ли --repos.
func needsMavenRepos(root string, opt Options) bool {
	isGradle := gradle.IsGradleProject(root)
	if isGradle && (!opt.SkipDeps || !opt.SkipPlugins) {
		return true
	}
	if isGradle && !opt.SkipCatalog {
		if _, err := os.Stat(catalogPath(root, opt.CatalogFile)); err == nil {
			return true
		}
	}
	if !opt.SkipPom && pom.Exists(root) {
		return true
	}
	// Wrapper ходит на distributionUrl из properties, а не в --repos.
	return false
}
