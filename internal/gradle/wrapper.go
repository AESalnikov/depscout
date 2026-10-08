package gradle

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// WrapperInfo описывает distributionUrl из gradle-wrapper.properties.
//
// Из URL вида https://host/.../gradle-8.12.1-bin.zip достаём версию, classifier
// и BaseURL (каталог distributions), чтобы потом искать более новый zip там же.
type WrapperInfo struct {
	Path            string // полный путь к файлу properties
	DistributionURL string // сырое значение (может содержать \:)
	Version         string // например "8.12.1"
	Classifier      string // "bin" или "all"
	BaseURL         string // каталог distributions без имени zip, без экранирования
}

var (
	distURLLineRe = regexp.MustCompile(`(?m)^(\s*distributionUrl\s*=\s*)(.+)$`)
	gradleZipRe   = regexp.MustCompile(`gradle-(\d+(?:\.\d+){0,2}(?:-[A-Za-z0-9.]+)?)-(bin|all)\.zip`)
)

// WrapperPropertiesPath возвращает стандартный путь
// <root>/gradle/wrapper/gradle-wrapper.properties.
func WrapperPropertiesPath(root string) string {
	return filepath.Join(root, "gradle", "wrapper", "gradle-wrapper.properties")
}

// ParseWrapper читает gradle-wrapper.properties и достаёт версию Gradle из URL.
func ParseWrapper(path string) (*WrapperInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseWrapperData(path, data)
}

// parseWrapperData разбирает уже прочитанные байты файла (удобно для Update без второго чтения).
func parseWrapperData(path string, data []byte) (*WrapperInfo, error) {
	m := distURLLineRe.FindSubmatch(data)
	if m == nil {
		return nil, fmt.Errorf("distributionUrl not found in %s", path)
	}
	raw := string(m[2])
	unescaped := unescapeGradleURL(raw)
	gm := gradleZipRe.FindStringSubmatch(unescaped)
	if gm == nil {
		return nil, fmt.Errorf("cannot parse gradle version from distributionUrl: %s", raw)
	}
	base := unescaped
	if idx := strings.LastIndex(unescaped, "/"); idx >= 0 {
		base = unescaped[:idx]
	}
	return &WrapperInfo{
		Path:            path,
		DistributionURL: raw,
		Version:         gm[1],
		Classifier:      gm[2],
		BaseURL:         base,
	}, nil
}

// unescapeGradleURL убирает обратные слэши Gradle (https\:// → https://).
func unescapeGradleURL(u string) string {
	return strings.ReplaceAll(u, `\`, "")
}

// escapeGradleURL экранирует ':' как в gradle-wrapper.properties (https\://...).
func escapeGradleURL(u string) string {
	return strings.ReplaceAll(u, ":", `\:`)
}

// UpdateWrapperVersion подставляет новую версию в distributionUrl.
// Тип дистрибутива (bin/all) и хост/путь каталога сохраняются.
func UpdateWrapperVersion(path, newVersion string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	info, err := parseWrapperData(path, data)
	if err != nil {
		return err
	}
	newURL := fmt.Sprintf("%s/gradle-%s-%s.zip", info.BaseURL, newVersion, info.Classifier)
	escaped := escapeGradleURL(newURL)

	out := distURLLineRe.ReplaceAllString(string(data), "${1}"+escaped)
	if out == string(data) {
		return nil
	}
	return os.WriteFile(path, []byte(out), 0o644)
}
