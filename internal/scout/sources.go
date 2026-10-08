package scout

import (
	"context"
	"os"
	"path/filepath"

	"github.com/AESalnikov/depscout/internal/gradle"
	"github.com/AESalnikov/depscout/internal/maven"
	"github.com/AESalnikov/depscout/internal/pom"
	"github.com/AESalnikov/depscout/internal/report"
	"github.com/AESalnikov/depscout/internal/wrapper"
)

// scoutPom проверяет зависимости и плагины из pom.xml.
func scoutPom(ctx context.Context, root string, client *maven.Client, claimed map[string]bool, upd *fileUpdates) ([]report.Row, error) {
	pomRefs, err := pom.Parse(pom.POMPath(root))
	if err != nil {
		return nil, err
	}
	var refs []VersionRef
	for _, r := range pomRefs {
		if claimed[r.GAV.String()] {
			continue
		}
		refs = append(refs, fromPom(r))
	}
	if len(refs) == 0 {
		return nil, nil
	}
	results := resolveVersionRefs(ctx, client, refs)
	var rows []report.Row
	for i, ref := range refs {
		claimed[ref.GAV.String()] = true
		rows = append(rows, rowFromRef(ref, results[i], func(latest string) {
			if ref.PropertyKey != "" {
				if prev, ok := upd.pomProps[ref.PropertyKey]; !ok || maven.Compare(latest, prev) > 0 {
					upd.pomProps[ref.PropertyKey] = latest
				}
			} else if ref.LiteralVersion {
				upd.pomLiterals[ref.GAV.String()] = latest
			}
		}))
	}
	return rows, nil
}

// scoutCatalog проверяет Version Catalog (libs.versions.toml).
func scoutCatalog(ctx context.Context, root string, client *maven.Client, catalogOverride string, claimed map[string]bool, upd *fileUpdates) ([]report.Row, error) {
	path := catalogPath(root, catalogOverride)
	catalogRefs, err := gradle.ParseCatalog(path)
	if err != nil {
		return nil, err
	}
	if len(catalogRefs) == 0 {
		return nil, nil
	}

	refs := make([]VersionRef, len(catalogRefs))
	for i, r := range catalogRefs {
		refs[i] = fromCatalog(r, path)
	}
	results := resolveVersionRefs(ctx, client, refs)

	var rows []report.Row
	for i, ref := range refs {
		claimed[ref.GAV.String()] = true
		r := catalogRefs[i]
		rows = append(rows, rowFromRef(ref, results[i], func(latest string) {
			if ref.CatalogVersionKey != "" {
				if prev, ok := upd.catalogVersions[ref.CatalogVersionKey]; !ok || maven.Compare(latest, prev) > 0 {
					upd.catalogVersions[ref.CatalogVersionKey] = latest
				}
			} else if r.LiteralOnEntry {
				upd.catalogLiterals[r.Alias] = latest
			}
		}))
	}
	return rows, nil
}

// scoutDeps сопоставляет gradle.properties с ${prop} в build-скриптах и резолвит latest.
func scoutDeps(ctx context.Context, root string, client *maven.Client, mapFile string, claimed map[string]bool, upd *fileUpdates) ([]report.Row, error) {
	propPath := filepath.Join(root, "gradle.properties")
	props, order, err := gradle.ParseProperties(propPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	byProp := map[string]gradle.GAV{}
	for _, script := range gradle.BuildScriptPaths(root) {
		refs, err := gradle.ParseDependencyProps(script)
		if err != nil {
			return nil, err
		}
		for k, v := range gradle.FirstGAVByProperty(refs) {
			if _, ok := byProp[k]; !ok {
				byProp[k] = v
			}
		}
	}

	if mapFile != "" {
		extra, err := loadMapFile(mapFile)
		if err != nil {
			return nil, err
		}
		for k, v := range extra {
			if _, ok := byProp[k]; !ok {
				byProp[k] = v
			}
		}
	}

	var refs []VersionRef
	var rows []report.Row

	for _, key := range order {
		val := props[key]
		if !gradle.IsVersionProperty(key, val) {
			continue
		}
		gav, mapped := byProp[key]
		if !mapped {
			rows = append(rows, report.Row{
				Kind:    report.KindProperty,
				Name:    key,
				Source:  "(unmapped)",
				Current: val,
				Status:  report.StatusSkip,
			})
			continue
		}
		if claimed[gav.String()] {
			continue // уже покрыто catalog
		}
		refs = append(refs, fromProperty(key, gav, val))
	}

	results := resolveVersionRefs(ctx, client, refs)
	for i, ref := range refs {
		claimed[ref.GAV.String()] = true
		rows = append(rows, rowFromRef(ref, results[i], func(latest string) {
			upd.props[ref.PropertyKey] = latest
		}))
	}

	return rows, nil
}

// scoutInline проверяет литеральные "g:a:1.2.3" в build-скриптах.
func scoutInline(ctx context.Context, root string, client *maven.Client, claimed map[string]bool, upd *fileUpdates) ([]report.Row, error) {
	var refs []VersionRef
	for _, script := range gradle.BuildScriptPaths(root) {
		deps, err := gradle.ParseInlineDependencies(script)
		if err != nil {
			return nil, err
		}
		for _, d := range deps {
			if claimed[d.GAV.String()] {
				continue
			}
			refs = append(refs, fromInline(d))
		}
	}
	if len(refs) == 0 {
		return nil, nil
	}

	results := resolveVersionRefs(ctx, client, refs)
	var rows []report.Row
	for i, ref := range refs {
		claimed[ref.GAV.String()] = true
		file := ref.File
		gavKey := ref.GAV.String()
		rows = append(rows, rowFromRef(ref, results[i], func(latest string) {
			m := upd.inlineByFile[file]
			if m == nil {
				m = map[string]string{}
				upd.inlineByFile[file] = m
			}
			m[gavKey] = latest
		}))
		// Source в отчёте — путь файла, не GAV (GAV уже в Name).
		rows[len(rows)-1].Source = file
	}
	return rows, nil
}

// scoutPlugins проверяет версии в блоке plugins { } через Plugin Marker GAV.
func scoutPlugins(ctx context.Context, root string, client *maven.Client, upd *fileUpdates) ([]report.Row, error) {
	var refs []VersionRef
	for _, script := range gradle.BuildScriptPaths(root) {
		ps, err := gradle.ParsePlugins(script)
		if err != nil {
			return nil, err
		}
		for _, p := range ps {
			refs = append(refs, fromPlugin(p))
		}
	}
	if len(refs) == 0 {
		return nil, nil
	}

	results := resolveVersionRefs(ctx, client, refs)
	var rows []report.Row
	for i, ref := range refs {
		file := ref.File
		id := ref.Name
		rows = append(rows, rowFromRef(ref, results[i], func(latest string) {
			m := upd.pluginsByFile[file]
			if m == nil {
				m = map[string]string{}
				upd.pluginsByFile[file] = m
			}
			m[id] = latest
		}))
	}
	return rows, nil
}

// scoutWrapper проверяет версию Gradle в distributionUrl.
// Возвращает строку отчёта и newVersion (непустую только если OUTDATED).
func scoutWrapper(ctx context.Context, root string, client *maven.Client, includePre bool, rewrites map[string]string) (report.Row, string, error) {
	path := gradle.WrapperPropertiesPath(root)
	info, err := gradle.ParseWrapper(path)
	if err != nil {
		return report.Row{}, "", err
	}
	baseURL := maven.RewriteURLHost(info.BaseURL, rewrites)
	r := &wrapper.Resolver{Client: client, IncludePreRelease: includePre}
	latest, err := r.LatestGradle(ctx, baseURL, info.Classifier)
	if err != nil {
		return report.Row{}, "", err
	}
	row := report.Row{
		Kind:    report.KindWrapper,
		Name:    "gradle",
		Source:  baseURL,
		Current: info.Version,
		Latest:  latest,
		Status:  report.StatusOK,
	}
	if maven.Compare(latest, info.Version) > 0 {
		row.Status = report.StatusOutdated
		return row, latest, nil
	}
	return row, "", nil
}
