package scout

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/AESalnikov/depscout/internal/gradle"
	"github.com/AESalnikov/depscout/internal/maven"
	"github.com/AESalnikov/depscout/internal/pom"
	"github.com/AESalnikov/depscout/internal/report"
)

// applyResolve заполняет Status/Latest у строки отчёта по результату Maven-resolve.
// onOutdated вызывается с новой версией, если статус OUTDATED (для накопления updates).
func applyResolve(row *report.Row, res maven.ResolveResult, current string, onOutdated func(latest string)) {
	switch {
	case res.Err != nil && !res.Found:
		row.Status = report.StatusError
		// Ошибку кладём в LATEST — иначе длинный GAV съедает truncate в COORDINATE.
		row.Latest = maven.ShortError(res.Err)
	case !res.Found:
		row.Status = report.StatusNotFound
	default:
		row.Latest = res.Latest
		if maven.Compare(res.Latest, current) > 0 {
			row.Status = report.StatusOutdated
			onOutdated(res.Latest)
		} else {
			row.Status = report.StatusOK
		}
	}
}

// applyUpdates пишет накопленные обновления в файлы проекта.
func applyUpdates(root, catalogOverride string, upd *fileUpdates) error {
	if len(upd.props) > 0 {
		if err := gradle.UpdatePropertiesValues(filepath.Join(root, "gradle.properties"), upd.props); err != nil {
			return fmt.Errorf("update gradle.properties: %w", err)
		}
	}
	for path, plugins := range upd.pluginsByFile {
		if err := gradle.UpdatePluginVersions(path, plugins); err != nil {
			return fmt.Errorf("update plugins in %s: %w", path, err)
		}
	}
	cat := catalogPath(root, catalogOverride)
	if len(upd.catalogVersions) > 0 {
		if err := gradle.UpdateCatalogVersions(cat, upd.catalogVersions); err != nil {
			return fmt.Errorf("update catalog versions: %w", err)
		}
	}
	if len(upd.catalogLiterals) > 0 {
		if err := gradle.UpdateCatalogEntryVersions(cat, upd.catalogLiterals); err != nil {
			return fmt.Errorf("update catalog entry versions: %w", err)
		}
	}
	for path, inlines := range upd.inlineByFile {
		if err := gradle.UpdateInlineVersions(path, inlines); err != nil {
			return fmt.Errorf("update inline deps in %s: %w", path, err)
		}
	}
	if len(upd.pomProps) > 0 {
		if err := pom.UpdateProperties(pom.POMPath(root), upd.pomProps); err != nil {
			return fmt.Errorf("update pom properties: %w", err)
		}
	}
	if len(upd.pomLiterals) > 0 {
		if err := pom.UpdateLiteralVersions(pom.POMPath(root), upd.pomLiterals); err != nil {
			return fmt.Errorf("update pom dependencies: %w", err)
		}
	}
	if upd.wrapper != "" {
		if err := gradle.UpdateWrapperVersion(gradle.WrapperPropertiesPath(root), upd.wrapper); err != nil {
			return fmt.Errorf("update wrapper: %w", err)
		}
	}
	return nil
}

// loadMapFile читает файл вида "prop=group:artifact" (по строке, # — комментарий).
func loadMapFile(path string) (map[string]gradle.GAV, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	out := make(map[string]gradle.GAV)
	sc := bufio.NewScanner(f)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			return nil, fmt.Errorf("%s:%d: expected prop=group:artifact", path, lineNo)
		}
		prop := strings.TrimSpace(line[:eq])
		coord := strings.TrimSpace(line[eq+1:])
		parts := strings.Split(coord, ":")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("%s:%d: expected prop=group:artifact", path, lineNo)
		}
		out[prop] = gradle.GAV{Group: parts[0], Artifact: parts[1]}
	}
	return out, sc.Err()
}
