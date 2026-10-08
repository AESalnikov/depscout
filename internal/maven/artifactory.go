package maven

import (
	"encoding/json"
	"net/url"
	"strings"
)

// splitArtifactoryRepo разбирает
//
//	https://host/artifactory/public → (https://host/artifactory, public, true)
func splitArtifactoryRepo(repo string) (root, repoKey string, ok bool) {
	repo = strings.TrimRight(strings.TrimSpace(repo), "/")
	const marker = "/artifactory/"
	i := strings.Index(repo, marker)
	if i < 0 {
		return "", "", false
	}
	root = repo[:i+len("/artifactory")]
	repoKey = strings.Trim(repo[i+len(marker):], "/")
	repoKey = strings.TrimPrefix(repoKey, "list/")
	repoKey = strings.Trim(repoKey, "/")
	if repoKey == "" || strings.Contains(repoKey, "/") {
		return "", "", false
	}
	return root, repoKey, true
}

// ArtifactoryLatestVersionURL — GET /api/search/latestVersion?g=&a=&repos= (text/plain).
func ArtifactoryLatestVersionURL(repo, group, artifact string) (string, bool) {
	root, repoKey, ok := splitArtifactoryRepo(repo)
	if !ok {
		return "", false
	}
	q := url.Values{}
	q.Set("g", group)
	q.Set("a", artifact)
	q.Set("repos", repoKey)
	return root + "/api/search/latestVersion?" + q.Encode(), true
}

// VersionFromArtifactoryLatestVersionBody разбирает text/plain ответ latestVersion.
func VersionFromArtifactoryLatestVersionBody(body []byte) string {
	s := strings.TrimSpace(string(body))
	if s == "" || s[0] == '{' || s[0] == '<' {
		return ""
	}
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	return s
}

// VersionsFromArtifactoryStorageJSON — папки-версии из JSON Index (Artifactory иногда
// отдаёт children вместо HTML).
func VersionsFromArtifactoryStorageJSON(body []byte) []string {
	body = trimJSONNoise(body)
	if len(body) == 0 || body[0] != '{' {
		return nil
	}
	var resp struct {
		Children []struct {
			URI string `json:"uri"`
		} `json:"children"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, ch := range resp.Children {
		name := strings.Trim(ch.URI, "/")
		if name == "" || strings.Contains(name, "/") || !looksLikeVersionDir(name) || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

func trimJSONNoise(b []byte) []byte {
	i := 0
	for i < len(b) && (b[i] == ' ' || b[i] == '\n' || b[i] == '\r' || b[i] == '\t') {
		i++
	}
	return b[i:]
}
