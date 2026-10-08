// Package wrapper ищет последнюю версию Gradle в каталоге distributions
// (тот же хост, что в distributionUrl проекта — не services.gradle.org принудительно).
package wrapper

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/AESalnikov/depscout/internal/maven"
)

// gradleDistRe ловит имена zip: gradle-8.12.1-bin.zip
var gradleDistRe = regexp.MustCompile(`gradle-(\d+(?:\.\d+){1,2})-(bin|all)\.zip`)

// Resolver находит latest Gradle distribution по base URL каталога.
type Resolver struct {
	Client            *maven.Client
	IncludePreRelease bool // зарезервировано для будущего расширения regex; сейчас zip-имена числовые
}

// LatestGradle делает GET {baseURL}/, парсит HTML/текст на имена zip и выбирает max версию
// с нужным classifier (bin/all).
func (r *Resolver) LatestGradle(ctx context.Context, baseURL, classifier string) (string, error) {
	baseURL = strings.TrimRight(baseURL, "/")
	body, status, err := r.Client.GetBytes(ctx, baseURL+"/")
	if err != nil {
		return "", err
	}
	if status != 200 {
		return "", fmt.Errorf("list %s/: HTTP %d", baseURL, status)
	}

	matches := gradleDistRe.FindAllStringSubmatch(string(body), -1)
	var versions []string
	seen := make(map[string]bool)
	for _, m := range matches {
		ver, dist := m[1], m[2]
		if classifier != "" && dist != classifier {
			continue
		}
		if seen[ver] {
			continue
		}
		seen[ver] = true
		versions = append(versions, ver)
	}
	if len(versions) == 0 {
		return "", fmt.Errorf("no gradle distributions found at %s", baseURL)
	}
	return maven.PickLatest(versions, r.IncludePreRelease), nil
}
