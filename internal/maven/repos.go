// Package maven ходит в Maven-совместимые репозитории (Artifactory, Nexus, Central):
// читает maven-metadata.xml и HTML Index, сравнивает версии, умеет Basic Auth.
package maven

import (
	"os"
	"strings"
)

// ParseRepoList разбирает строку "url1,url2,..." в список base URL репозиториев.
// Хвостовые '/' срезаются, дубликаты отбрасываются (порядок — первое вхождение).
func ParseRepoList(spec string) []string {
	var out []string
	seen := make(map[string]bool)
	for _, part := range strings.Split(spec, ",") {
		u := strings.TrimRight(strings.TrimSpace(part), "/")
		if u == "" || seen[u] {
			continue
		}
		seen[u] = true
		out = append(out, u)
	}
	return out
}

// ReposFromEnv читает DEPSCOUT_REPOS (список URL через запятую).
func ReposFromEnv() []string {
	return ParseRepoList(os.Getenv("DEPSCOUT_REPOS"))
}

// MergeRepos склеивает несколько списков репозиториев без дубликатов.
// Первое появление URL побеждает (важно, если один и тот же mirror задан и в env, и в флаге).
func MergeRepos(lists ...[]string) []string {
	var out []string
	seen := make(map[string]bool)
	for _, list := range lists {
		for _, u := range list {
			u = strings.TrimRight(strings.TrimSpace(u), "/")
			if u == "" || seen[u] {
				continue
			}
			seen[u] = true
			out = append(out, u)
		}
	}
	return out
}
