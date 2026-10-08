package gradle

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// GAV — Maven-координата group:artifact (версия хранится отдельно).
type GAV struct {
	Group    string // например "io.orbitcart.core"
	Artifact string // например "orbit-core"
}

// String возвращает "group:artifact".
func (g GAV) String() string {
	return g.Group + ":" + g.Artifact
}

// DependencyRef связывает имя свойства из gradle.properties с GAV из build-скрипта.
// Пример: Property=orbitCoreVersion, GAV=io.orbitcart.core:orbit-core.
type DependencyRef struct {
	Property string
	GAV      GAV
}

// InlineDependency — зависимость с захардкоженной версией: "g:a:1.2.3".
type InlineDependency struct {
	GAV     GAV
	Version string
	File    string
}

// PluginRef — плагин из блока plugins { id "..." version "..." } (Groovy или Kotlin DSL).
type PluginRef struct {
	ID      string // plugin id, например "io.orbitcart.conventions"
	Version string // текущая версия из build-скрипта
	GAV     GAV    // Gradle Plugin Marker для lookup в Maven-репозитории
	File    string
}

var (
	// "group:artifact:${prop}" или с одинарными кавычками (Groovy и Kotlin DSL)
	depWithPropRe = regexp.MustCompile(
		`['"]([A-Za-z0-9_.\-]+):([A-Za-z0-9_.\-]+):\$\{([A-Za-z0-9_.\-]+)}['"]`,
	)
	// литерал "group:artifact:1.2.3" — версия без ${}
	depInlineRe = regexp.MustCompile(
		`['"]([A-Za-z0-9_.\-]+):([A-Za-z0-9_.\-]+):([^'"$\{]+)['"]`,
	)
	// Groovy: id "x" version "y"
	pluginLineGroovyRe = regexp.MustCompile(
		`(?m)^\s*id\s+['"]([^'"]+)['"]\s+version\s+['"]([^'"]+)['"]`,
	)
	// Kotlin: id("x") version "y"
	pluginLineKtsRe = regexp.MustCompile(
		`(?m)^\s*id\s*\(\s*["']([^"']+)["']\s*\)\s*version\s+["']([^"']+)["']`,
	)
	// Kotlin: id("x") version("y")
	pluginLineKtsParenRe = regexp.MustCompile(
		`(?m)^\s*id\s*\(\s*["']([^"']+)["']\s*\)\s*version\s*\(\s*["']([^"']+)["']\s*\)`,
	)
	pluginsBlockRe = regexp.MustCompile(`(?s)plugins\s*\{(.*?)}`)
)

// ParseDependencyProps извлекает из build-скрипта связи ${свойство} → GAV.
//
// Ищет литералы вида "g:a:${prop}". Named arguments (group:, name:) не поддерживаются.
func ParseDependencyProps(path string) ([]DependencyRef, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	matches := depWithPropRe.FindAllStringSubmatch(string(data), -1)
	var refs []DependencyRef
	seen := make(map[string]bool)
	for _, m := range matches {
		ref := DependencyRef{
			Property: m[3],
			GAV:      GAV{Group: m[1], Artifact: m[2]},
		}
		key := ref.Property + "|" + ref.GAV.String()
		if seen[key] {
			continue
		}
		seen[key] = true
		refs = append(refs, ref)
	}
	return refs, nil
}

// ParseInlineDependencies извлекает зависимости с литеральной версией "g:a:1.2.3".
//
// Пропускает dynamic-версии (содержат "+", "latest.") и строки с ${}.
func ParseInlineDependencies(path string) ([]InlineDependency, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	matches := depInlineRe.FindAllStringSubmatch(string(data), -1)
	var refs []InlineDependency
	seen := make(map[string]bool)
	for _, m := range matches {
		ver := strings.TrimSpace(m[3])
		// regex уже исключает ${...}; отсекаем только dynamic (1.+, latest.*)
		if isDynamicVersion(ver) {
			continue
		}
		ref := InlineDependency{
			GAV:     GAV{Group: m[1], Artifact: m[2]},
			Version: ver,
			File:    path,
		}
		key := ref.GAV.String() + "|" + ref.Version
		if seen[key] {
			continue
		}
		seen[key] = true
		refs = append(refs, ref)
	}
	return refs, nil
}

// isDynamicVersion — версии Gradle вроде 1.+ / latest.release не резолвим как релизы.
func isDynamicVersion(v string) bool {
	lower := strings.ToLower(v)
	return strings.Contains(v, "+") || strings.HasPrefix(lower, "latest.")
}

// FirstGAVByProperty оставляет первый GAV для каждого свойства.
// Если одно свойство использовано с разными артефактами — берём первое вхождение.
func FirstGAVByProperty(refs []DependencyRef) map[string]GAV {
	out := make(map[string]GAV)
	for _, r := range refs {
		if _, ok := out[r.Property]; ok {
			continue
		}
		out[r.Property] = r.GAV
	}
	return out
}

// ParsePlugins читает первый блок plugins { ... } (Groovy или Kotlin DSL).
//
// Плагины без version игнорируются. Повторный id тоже игнорируется (первое wins).
// Если блока нет — возвращает (nil, nil), это не ошибка.
func ParsePlugins(path string) ([]PluginRef, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block := pluginsBlockRe.FindStringSubmatch(string(data))
	if block == nil {
		return nil, nil
	}
	body := block[1]
	var refs []PluginRef
	seen := make(map[string]bool)

	add := func(id, ver string) {
		if seen[id] {
			return
		}
		seen[id] = true
		refs = append(refs, PluginRef{
			ID:      id,
			Version: ver,
			GAV:     PluginMarkerGAV(id),
			File:    path,
		})
	}
	for _, m := range pluginLineGroovyRe.FindAllStringSubmatch(body, -1) {
		add(m[1], m[2])
	}
	for _, m := range pluginLineKtsRe.FindAllStringSubmatch(body, -1) {
		add(m[1], m[2])
	}
	for _, m := range pluginLineKtsParenRe.FindAllStringSubmatch(body, -1) {
		add(m[1], m[2])
	}
	return refs, nil
}

// PluginMarkerGAV строит Maven-координаты Gradle Plugin Marker.
//
// Соглашение Gradle: id "a.b.c" → group=a.b.c, artifact=a.b.c.gradle.plugin.
// Именно так portal/Artifactory отдают maven-metadata.xml для плагинов.
func PluginMarkerGAV(pluginID string) GAV {
	return GAV{
		Group:    pluginID,
		Artifact: pluginID + ".gradle.plugin",
	}
}

// UpdatePluginVersions меняет version у указанных plugin id в блоке plugins {}.
// Поддерживает Groovy и Kotlin DSL. updates: map[pluginId]newVersion.
func UpdatePluginVersions(path string, updates map[string]string) error {
	if len(updates) == 0 {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	content := string(data)
	loc := pluginsBlockRe.FindStringSubmatchIndex(content)
	if loc == nil {
		return fmt.Errorf("plugins block not found in %s", path)
	}
	blockStart, blockEnd := loc[2], loc[3]
	block := content[blockStart:blockEnd]

	groovyLine := regexp.MustCompile(`(?m)^(\s*id\s+['"])([^'"]+)(['"]\s+version\s+['"])([^'"]+)(['"].*)$`)
	ktsLine := regexp.MustCompile(`(?m)^(\s*id\s*\(\s*["'])([^"']+)(["']\s*\)\s+version\s+["'])([^"']+)(["'].*)$`)
	ktsParenLine := regexp.MustCompile(`(?m)^(\s*id\s*\(\s*["'])([^"']+)(["']\s*\)\s+version\s*\(\s*["'])([^"']+)(["']\s*\).*)$`)

	replacePluginLine := func(re *regexp.Regexp, block string) string {
		return re.ReplaceAllStringFunc(block, func(line string) string {
			m := re.FindStringSubmatch(line)
			id := m[2]
			newVer, ok := updates[id]
			if !ok {
				return line
			}
			return m[1] + id + m[3] + newVer + m[5]
		})
	}
	newBlock := replacePluginLine(groovyLine, block)
	newBlock = replacePluginLine(ktsLine, newBlock)
	newBlock = replacePluginLine(ktsParenLine, newBlock)

	if newBlock == block {
		return nil
	}
	out := content[:blockStart] + newBlock + content[blockEnd:]
	return os.WriteFile(path, []byte(out), 0o644)
}

// UpdateInlineVersions заменяет литеральные "g:a:old" на "g:a:new".
// Ключ updates — "group:artifact" (первая подходящая литеральная версия).
func UpdateInlineVersions(path string, updates map[string]string) error {
	if len(updates) == 0 {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	content := string(data)
	changed := false
	out := depInlineRe.ReplaceAllStringFunc(content, func(match string) string {
		m := depInlineRe.FindStringSubmatch(match)
		gav := m[1] + ":" + m[2]
		newVer, ok := updates[gav]
		if !ok {
			return match
		}
		ver := strings.TrimSpace(m[3])
		if isDynamicVersion(ver) {
			return match
		}
		changed = true
		quote := match[0:1]
		return quote + gav + ":" + newVer + quote
	})
	if !changed {
		return nil
	}
	return os.WriteFile(path, []byte(out), 0o644)
}
