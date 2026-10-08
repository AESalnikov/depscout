package gradle

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// CatalogRef — зависимость или плагин из libs.versions.toml.
type CatalogRef struct {
	Alias          string // ключ в [libraries] или [plugins]
	Section        string // "library" | "plugin"
	GAV            GAV
	Current        string
	VersionKey     string // ключ в [versions], если version.ref
	LiteralOnEntry bool   // version задан прямо на library/plugin
}

// ParseCatalog читает Version Catalog (TOML).
//
// Поддерживает типичные формы:
//
//	[versions]
//	orbitCore = "2.4.1"
//
//	[libraries]
//	orbit-core = { module = "io.orbitcart.core:orbit-core", version.ref = "orbitCore" }
//	other = { group = "com.ex", name = "lib", version = "1.0.0" }
//
//	[plugins]
//	conventions = { id = "io.orbitcart.conventions", version.ref = "orbitCore" }
//
// Если файла нет — (nil, nil). Bundles не разбираются.
func ParseCatalog(path string) ([]CatalogRef, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return parseCatalogTOML(path, data)
}

type rawCatalog struct {
	Versions  map[string]any            `toml:"versions"`
	Libraries map[string]map[string]any `toml:"libraries"`
	Plugins   map[string]map[string]any `toml:"plugins"`
}

func parseCatalogTOML(path string, data []byte) ([]CatalogRef, error) {
	var raw rawCatalog
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	versions := map[string]string{}
	for k, v := range raw.Versions {
		if s, ok := anyString(v); ok {
			versions[k] = s
		}
	}

	var refs []CatalogRef
	for alias, table := range raw.Libraries {
		ref, ok := catalogLibraryRef(alias, table, versions)
		if ok {
			refs = append(refs, ref)
		}
	}
	for alias, table := range raw.Plugins {
		ref, ok := catalogPluginRef(alias, table, versions)
		if ok {
			refs = append(refs, ref)
		}
	}
	return refs, nil
}

func catalogLibraryRef(alias string, table map[string]any, versions map[string]string) (CatalogRef, bool) {
	group, artifact := "", ""
	if mod, ok := anyString(table["module"]); ok {
		parts := strings.SplitN(mod, ":", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return CatalogRef{}, false
		}
		group, artifact = parts[0], parts[1]
	} else {
		g, gok := anyString(table["group"])
		n, nok := anyString(table["name"])
		if !gok || !nok {
			return CatalogRef{}, false
		}
		group, artifact = g, n
	}
	ver, verKey, literal, ok := resolveCatalogVersion(table, versions)
	if !ok {
		return CatalogRef{}, false
	}
	return CatalogRef{
		Alias:          alias,
		Section:        "library",
		GAV:            GAV{Group: group, Artifact: artifact},
		Current:        ver,
		VersionKey:     verKey,
		LiteralOnEntry: literal,
	}, true
}

func catalogPluginRef(alias string, table map[string]any, versions map[string]string) (CatalogRef, bool) {
	id, ok := anyString(table["id"])
	if !ok || id == "" {
		return CatalogRef{}, false
	}
	ver, verKey, literal, ok := resolveCatalogVersion(table, versions)
	if !ok {
		return CatalogRef{}, false
	}
	return CatalogRef{
		Alias:          alias,
		Section:        "plugin",
		GAV:            PluginMarkerGAV(id),
		Current:        ver,
		VersionKey:     verKey,
		LiteralOnEntry: literal,
	}, true
}

// resolveCatalogVersion достаёт текущую версию из version.ref или version.
//
// В TOML запись version.ref внутри inline-таблицы часто приходит как вложенный
// map: version = { ref = "..." } (так делает go-toml).
func resolveCatalogVersion(table map[string]any, versions map[string]string) (ver, verKey string, literal, ok bool) {
	if ref, rok := anyString(table["version.ref"]); rok && ref != "" {
		return lookupVersionRef(ref, versions)
	}
	switch v := table["version"].(type) {
	case map[string]any:
		if ref, rok := anyString(v["ref"]); rok && ref != "" {
			return lookupVersionRef(ref, versions)
		}
	default:
		if s, sok := anyString(v); sok && s != "" {
			return s, "", true, true
		}
	}
	return "", "", false, false
}

func lookupVersionRef(ref string, versions map[string]string) (ver, verKey string, literal, ok bool) {
	v, found := versions[ref]
	if !found || v == "" {
		return "", "", false, false
	}
	return v, ref, false, true
}

func anyString(v any) (string, bool) {
	switch t := v.(type) {
	case string:
		return t, true
	case int64:
		return fmt.Sprintf("%d", t), true
	case float64:
		return fmt.Sprintf("%v", t), true
	default:
		return "", false
	}
}

var (
	// versionKeyRe — строка в [versions]: key = "1.2.3"
	versionKeyRe = regexp.MustCompile(`(?m)^(\s*)([A-Za-z0-9_.\-]+)(\s*=\s*")([^"]*)("\s*)$`)
)

// UpdateCatalogVersions обновляет значения ключей в секции [versions].
// updates: map[versionKey]newVersion. Пустой — no-op.
func UpdateCatalogVersions(path string, updates map[string]string) error {
	if len(updates) == 0 {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	content := string(data)
	section := catalogSectionSpan(content, "versions")
	if section.start < 0 {
		return fmt.Errorf("%s: [versions] section not found", path)
	}
	body := content[section.start:section.end]
	changed := false
	newBody := versionKeyRe.ReplaceAllStringFunc(body, func(line string) string {
		m := versionKeyRe.FindStringSubmatch(line)
		key := m[2]
		newVer, ok := updates[key]
		if !ok {
			return line
		}
		changed = true
		return m[1] + key + m[3] + newVer + m[5]
	})
	if !changed {
		return nil
	}
	out := content[:section.start] + newBody + content[section.end:]
	return os.WriteFile(path, []byte(out), 0o644)
}

// UpdateCatalogEntryVersions обновляет литеральные version = "..." у записей
// [libraries]/[plugins] по alias. Ключ updates — alias.
func UpdateCatalogEntryVersions(path string, updates map[string]string) error {
	if len(updates) == 0 {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	content := string(data)
	changed := false
	for _, sec := range []string{"libraries", "plugins"} {
		span := catalogSectionSpan(content, sec)
		if span.start < 0 {
			continue
		}
		body := content[span.start:span.end]
		newBody := rewriteCatalogLiteralVersions(body, updates, &changed)
		content = content[:span.start] + newBody + content[span.end:]
		// пересчитать span после изменения длины — проще перечитать spans с начала на следующем круге;
		// т.к. мы идём по секциям последовательно и меняем content, следующий span ищем в уже обновлённом content.
		_ = span
	}
	if !changed {
		return nil
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func rewriteCatalogLiteralVersions(body string, updates map[string]string, changed *bool) string {
	lineRe := regexp.MustCompile(`(?m)^(\s*)([A-Za-z0-9_.\-]+)(\s*=\s*\{)([^}]*)(\}\s*)$`)
	return lineRe.ReplaceAllStringFunc(body, func(line string) string {
		m := lineRe.FindStringSubmatch(line)
		alias := m[2]
		newVer, ok := updates[alias]
		if !ok {
			return line
		}
		inner := m[4]
		// только литеральный version=, не version.ref
		if strings.Contains(inner, "version.ref") {
			return line
		}
		verRe := regexp.MustCompile(`(version\s*=\s*")([^"]*)(")`)
		if !verRe.MatchString(inner) {
			return line
		}
		*changed = true
		inner = verRe.ReplaceAllString(inner, `${1}`+newVer+`${3}`)
		return m[1] + alias + m[3] + inner + m[5]
	})
}

type sectionSpan struct{ start, end int }

// catalogSectionSpan находит тело секции [name] до следующей [section] или EOF.
func catalogSectionSpan(content, name string) sectionSpan {
	header := regexp.MustCompile(`(?m)^\s*\[\s*` + regexp.QuoteMeta(name) + `\s*\]\s*$`)
	loc := header.FindStringIndex(content)
	if loc == nil {
		return sectionSpan{-1, -1}
	}
	start := loc[1]
	// пропустить перевод строки после заголовка (\n или \r\n)
	for start < len(content) && (content[start] == '\n' || content[start] == '\r') {
		start++
	}
	next := regexp.MustCompile(`(?m)^\s*\[[^\]]+\]\s*$`)
	rest := content[start:]
	if nloc := next.FindStringIndex(rest); nloc != nil {
		return sectionSpan{start, start + nloc[0]}
	}
	return sectionSpan{start, len(content)}
}
