// Package pom читает и точечно обновляет Maven pom.xml.
//
// Поддерживаются:
//   - <properties><x.version>1.2.3</x.version>
//   - <dependency> / <dependencyManagement> с ${prop} или литеральной версией
//   - <plugin> с версией (в т.ч. в pluginManagement)
//
// Не резолвит parent POM по сети и не раскрывает BOM import — только то, что есть в файле.
package pom

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/AESalnikov/depscout/internal/gradle"
	"github.com/AESalnikov/depscout/internal/report"
)

// Kind — алиасы report.Kind для pom-источников.
const (
	KindProperty = report.KindPom
	KindDep      = report.KindPomDep
	KindPlugin   = report.KindPomPlugin
)

// Ref — одна координата из pom.xml.
type Ref struct {
	Kind           report.Kind
	Name           string // имя property или group:artifact
	GAV            gradle.GAV
	Current        string
	PropertyKey    string // если версия через ${key}
	LiteralVersion bool   // версия захардкожена в dependency/plugin
}

// Exists — есть ли pom.xml в корне.
func Exists(root string) bool {
	_, err := os.Stat(POMPath(root))
	return err == nil
}

// POMPath возвращает <root>/pom.xml.
func POMPath(root string) string {
	return filepath.Join(root, "pom.xml")
}

type rawProject struct {
	XMLName              xml.Name `xml:"project"`
	Properties           rawProps `xml:"properties"`
	Dependencies         rawDeps  `xml:"dependencies"`
	DependencyManagement struct {
		Dependencies rawDeps `xml:"dependencies"`
	} `xml:"dependencyManagement"`
	Build struct {
		Plugins          rawPlugins `xml:"plugins"`
		PluginManagement struct {
			Plugins rawPlugins `xml:"plugins"`
		} `xml:"pluginManagement"`
	} `xml:"build"`
}

type rawProps struct {
	Inner []byte `xml:",innerxml"`
}

type rawDeps struct {
	Dependency []rawDep `xml:"dependency"`
}

type rawDep struct {
	GroupID    string `xml:"groupId"`
	ArtifactID string `xml:"artifactId"`
	Version    string `xml:"version"`
}

type rawPlugins struct {
	Plugin []rawPlugin `xml:"plugin"`
}

type rawPlugin struct {
	GroupID    string `xml:"groupId"`
	ArtifactID string `xml:"artifactId"`
	Version    string `xml:"version"`
}

var propTagRe = regexp.MustCompile(`(?s)<([A-Za-z_][\w.-]*)>\s*([^<]*?)\s*</([A-Za-z_][\w.-]*)>`)

// Parse читает pom.xml и возвращает проверяемые версии.
func Parse(path string) ([]Ref, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var proj rawProject
	if err := xml.Unmarshal(data, &proj); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	props := parseProperties(proj.Properties.Inner)
	var refs []Ref
	seen := map[string]bool{}

	addDep := func(d rawDep, kind string) {
		if d.GroupID == "" || d.ArtifactID == "" || d.Version == "" {
			return
		}
		if strings.HasPrefix(strings.TrimSpace(d.Version), "${") {
			key := propertyKey(d.Version)
			if key == "" {
				return
			}
			cur, ok := props[key]
			if !ok || cur == "" {
				return
			}
			id := kind + "|prop|" + key + "|" + d.GroupID + ":" + d.ArtifactID
			if seen[id] {
				return
			}
			seen[id] = true
			refs = append(refs, Ref{
				Kind:        KindProperty,
				Name:        key,
				GAV:         gradle.GAV{Group: d.GroupID, Artifact: d.ArtifactID},
				Current:     cur,
				PropertyKey: key,
			})
			return
		}
		if isSkippedVersion(d.Version) {
			return
		}
		gav := d.GroupID + ":" + d.ArtifactID
		id := kind + "|lit|" + gav
		if seen[id] {
			return
		}
		seen[id] = true
		refs = append(refs, Ref{
			Kind:           KindDep,
			Name:           gav,
			GAV:            gradle.GAV{Group: d.GroupID, Artifact: d.ArtifactID},
			Current:        strings.TrimSpace(d.Version),
			LiteralVersion: true,
		})
	}

	for _, d := range proj.Dependencies.Dependency {
		addDep(d, "dep")
	}
	for _, d := range proj.DependencyManagement.Dependencies.Dependency {
		addDep(d, "dm")
	}

	addPlugin := func(p rawPlugin) {
		if p.ArtifactID == "" || p.Version == "" {
			return
		}
		group := p.GroupID
		if group == "" {
			group = "org.apache.maven.plugins"
		}
		if strings.HasPrefix(strings.TrimSpace(p.Version), "${") {
			key := propertyKey(p.Version)
			if key == "" {
				return
			}
			cur, ok := props[key]
			if !ok || cur == "" {
				return
			}
			id := "plugin|prop|" + key + "|" + group + ":" + p.ArtifactID
			if seen[id] {
				return
			}
			seen[id] = true
			refs = append(refs, Ref{
				Kind:        KindProperty,
				Name:        key,
				GAV:         gradle.GAV{Group: group, Artifact: p.ArtifactID},
				Current:     cur,
				PropertyKey: key,
			})
			return
		}
		if isSkippedVersion(p.Version) {
			return
		}
		gav := group + ":" + p.ArtifactID
		id := "plugin|lit|" + gav
		if seen[id] {
			return
		}
		seen[id] = true
		refs = append(refs, Ref{
			Kind:           KindPlugin,
			Name:           gav,
			GAV:            gradle.GAV{Group: group, Artifact: p.ArtifactID},
			Current:        strings.TrimSpace(p.Version),
			LiteralVersion: true,
		})
	}

	for _, p := range proj.Build.Plugins.Plugin {
		addPlugin(p)
	}
	for _, p := range proj.Build.PluginManagement.Plugins.Plugin {
		addPlugin(p)
	}

	return refs, nil
}

func parseProperties(inner []byte) map[string]string {
	out := map[string]string{}
	for _, m := range propTagRe.FindAllSubmatch(inner, -1) {
		open, val, close := string(m[1]), string(m[2]), string(m[3])
		if open != close {
			continue
		}
		out[open] = strings.TrimSpace(val)
	}
	return out
}

func propertyKey(ver string) string {
	ver = strings.TrimSpace(ver)
	if !strings.HasPrefix(ver, "${") || !strings.HasSuffix(ver, "}") {
		return ""
	}
	return strings.TrimSpace(ver[2 : len(ver)-1])
}

func isSkippedVersion(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" {
		return true
	}
	lower := strings.ToLower(v)
	return strings.Contains(v, "+") || strings.HasPrefix(lower, "latest.") || strings.Contains(v, ",")
}

// UpdateProperties меняет <key>old</key> внутри <properties>.
func UpdateProperties(path string, updates map[string]string) error {
	if len(updates) == 0 {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	content := string(data)
	changed := false
	for key, newVer := range updates {
		re := regexp.MustCompile(`(<` + regexp.QuoteMeta(key) + `>)([^<]*)(</` + regexp.QuoteMeta(key) + `>)`)
		if !re.MatchString(content) {
			continue
		}
		content = re.ReplaceAllString(content, `${1}`+newVer+`${3}`)
		changed = true
	}
	if !changed {
		return nil
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// UpdateLiteralVersions обновляет <version> у dependency/plugin с литеральной версией.
// Ключ updates — "group:artifact".
func UpdateLiteralVersions(path string, updates map[string]string) error {
	if len(updates) == 0 {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	content := string(data)
	changed := false
	for gav, newVer := range updates {
		parts := strings.SplitN(gav, ":", 2)
		if len(parts) != 2 {
			continue
		}
		group, artifact := parts[0], parts[1]
		// блок dependency или plugin, содержащий groupId+artifactId, затем version не-${}
		re := regexp.MustCompile(
			`(?s)(<groupId>\s*` + regexp.QuoteMeta(group) + `\s*</groupId>\s*` +
				`<artifactId>\s*` + regexp.QuoteMeta(artifact) + `\s*</artifactId>\s*` +
				`<version>)([^<$]+)(</version>)`,
		)
		if !re.MatchString(content) {
			// version может быть до artifactId — редкий порядок; попробуем artifact затем group
			re = regexp.MustCompile(
				`(?s)(<artifactId>\s*` + regexp.QuoteMeta(artifact) + `\s*</artifactId>\s*` +
					`<groupId>\s*` + regexp.QuoteMeta(group) + `\s*</groupId>\s*` +
					`<version>)([^<$]+)(</version>)`,
			)
		}
		if !re.MatchString(content) {
			continue
		}
		content = re.ReplaceAllString(content, `${1}`+newVer+`${3}`)
		changed = true
	}
	if !changed {
		return nil
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
