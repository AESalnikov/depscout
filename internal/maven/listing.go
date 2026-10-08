package maven

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// hrefRe ловит любые href в HTML Index Artifactory/Nexus/Central.
// Дальше из значения достаём последний сегмент пути (версию).
var hrefRe = regexp.MustCompile(`(?i)href=["']([^"']+)["']`)

// bareVersionDirRe — запасной разбор «Index of» без нормальных href:
// находит токены вида 10.1.3/ в тексте страницы (Artifactory 7 HTML бывает капризным).
var bareVersionDirRe = regexp.MustCompile(`(?:^|[\s"'>=])(\d+(?:\.\d+){1,4}(?:-[A-Za-z0-9._]+)?)(/)(?:[\s"'<]|$)`)

// ArtifactDirURL строит URL каталога артефакта {repo}/{group.path}/{artifact}/.
func ArtifactDirURL(repo, group, artifact string) string {
	repo = strings.TrimRight(repo, "/")
	groupPath := strings.ReplaceAll(group, ".", "/")
	return fmt.Sprintf("%s/%s/%s/", repo, groupPath, artifact)
}

// VersionsFromHTMLListing вытаскивает имена версий из HTML-листинга репозитория.
//
// Поддерживает:
//   - относительные: href="1.2.3/" и href="1.2.3"
//   - абсолютные: href="https://host/.../artifact/1.2.3/"
//
// Нужен как fallback, когда maven-metadata.xml битый/устаревший (часто в Artifactory).
func VersionsFromHTMLListing(body string) []string {
	matches := hrefRe.FindAllStringSubmatch(body, -1)
	seen := make(map[string]bool)
	var out []string
	for _, m := range matches {
		name := versionNameFromHref(m[1])
		if name == "" || name == ".." || strings.Contains(name, "maven-metadata") {
			continue
		}
		if strings.Contains(name, ".") && !looksLikeVersionDir(name) {
			lower := strings.ToLower(name)
			if strings.HasSuffix(lower, ".xml") ||
				strings.HasSuffix(lower, ".md5") ||
				strings.HasSuffix(lower, ".sha1") ||
				strings.HasSuffix(lower, ".sha256") ||
				strings.HasSuffix(lower, ".sha512") ||
				strings.HasSuffix(lower, ".html") ||
				strings.HasSuffix(lower, ".htm") {
				continue
			}
		}
		if !looksLikeVersionDir(name) {
			continue
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	// Дополнительно: токены «10.1.3/» из текста Index of (когда href битые/JS).
	for _, m := range bareVersionDirRe.FindAllStringSubmatch(body, -1) {
		name := m[1]
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

// versionNameFromHref достаёт имя каталога версии из href (относительного или абсолютного).
func versionNameFromHref(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "#") || isExecutableURLScheme(raw) {
		return ""
	}
	if i := strings.IndexAny(raw, "?#"); i >= 0 {
		raw = raw[:i]
	}
	// url.Parse понимает и path-only, и absolute
	if u, err := url.Parse(raw); err == nil && u.Path != "" {
		raw = u.Path
	}
	raw = strings.TrimSuffix(raw, "/")
	if i := strings.LastIndex(raw, "/"); i >= 0 {
		raw = raw[i+1:]
	}
	return raw
}

// isExecutableURLScheme отклоняет javascript:/data:/vbscript: (и регистровые варианты).
func isExecutableURLScheme(raw string) bool {
	lower := strings.ToLower(raw)
	return strings.HasPrefix(lower, "javascript:") ||
		strings.HasPrefix(lower, "data:") ||
		strings.HasPrefix(lower, "vbscript:")
}

// looksLikeVersionDir — эвристика: в имени каталога должна быть хотя бы одна цифра.
func looksLikeVersionDir(name string) bool {
	for _, r := range name {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
}
