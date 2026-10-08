package maven

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/AESalnikov/depscout/internal/gradle"
)

// Client — HTTP-клиент для запросов maven-metadata и HTML Index.
// Поля экспортированы, чтобы тесты могли подставить httptest.Server и свой Timeout.
type Client struct {
	HTTP              *http.Client // если nil в тестах — задайте явно; NewClient создаёт с Timeout
	Repos             []string     // base URL репозиториев
	User              string       // Basic Auth user (может быть пустым)
	Password          string
	IncludePreRelease bool      // если false — только числовые релизы
	Concurrency       int       // размер пула горутин для ResolveMany; <=0 → 8
	Debug             io.Writer // если не nil — лог HTTP/resolve (--debug / DEPSCOUT_DEBUG)
}

// AuthFromEnv читает логин/пароль из окружения.
// Приоритет: DEPSCOUT_* , затем ARTIFACTORY_*.
func AuthFromEnv() (user, pass string) {
	user = firstNonEmpty(os.Getenv("DEPSCOUT_USER"), os.Getenv("ARTIFACTORY_USER"))
	pass = firstNonEmpty(os.Getenv("DEPSCOUT_PASSWORD"), os.Getenv("ARTIFACTORY_PASSWORD"))
	return user, pass
}

// firstNonEmpty возвращает первую непустую строку из списка (или "").
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// NewClient создаёт клиент с таймаутом 15s, auth из env и concurrency=8.
func NewClient(repos []string, includePreRelease bool) *Client {
	user, pass := AuthFromEnv()
	c := &Client{
		HTTP:              &http.Client{Timeout: 15 * time.Second},
		Repos:             repos,
		User:              user,
		Password:          pass,
		IncludePreRelease: includePreRelease,
		Concurrency:       8,
	}
	if os.Getenv("DEPSCOUT_DEBUG") != "" {
		c.Debug = os.Stderr
	}
	return c
}

func (c *Client) debugf(format string, args ...any) {
	if c == nil || c.Debug == nil {
		return
	}
	_, _ = fmt.Fprintf(c.Debug, "depscout: "+format+"\n", args...)
}

// ResolveResult — итог поиска latest для одного GAV.
type ResolveResult struct {
	GAV    gradle.GAV
	Latest string // найденная версия (если Found)
	Repo   string // какой base URL сработал
	Err    error  // последняя сетевая/HTTP ошибка, если не нашли
	Found  bool
}

// ResolveLatest ищет latest версию gav по всем Repos по порядку.
// Первый успешный репозиторий побеждает.
//
// Если репозиторий ответил «пусто» (нет релизных версий) — это not-found,
// а не ERROR. Ошибки следующих репо (404/auth) не должны перекрывать
// успешный пустой ответ с первого зеркала (типичный случай: public + plugins).
func (c *Client) ResolveLatest(ctx context.Context, gav gradle.GAV) ResolveResult {
	c.debugf("resolve %s", gav)
	var lastErr error
	var sawNotFound bool
	for _, repo := range c.Repos {
		latest, err := c.resolveInRepo(ctx, repo, gav)
		if err != nil {
			c.debugf("  repo %s: err=%v", repo, err)
			lastErr = err
			continue
		}
		if latest == "" {
			c.debugf("  repo %s: empty", repo)
			sawNotFound = true
			continue
		}
		c.debugf("  repo %s: latest=%s", repo, latest)
		return ResolveResult{GAV: gav, Latest: latest, Repo: repo, Found: true}
	}
	if sawNotFound {
		return ResolveResult{GAV: gav, Found: false}
	}
	if lastErr != nil {
		return ResolveResult{GAV: gav, Err: lastErr, Found: false}
	}
	return ResolveResult{GAV: gav, Err: fmt.Errorf("no repositories configured"), Found: false}
}

// resolveInRepo объединяет версии из maven-metadata.xml и HTML Index одного репозитория.
func (c *Client) resolveInRepo(ctx context.Context, repo string, gav gradle.GAV) (string, error) {
	var versions []string
	var metaErr error

	metaURL := MetadataURL(repo, gav.Group, gav.Artifact)
	body, status, err := c.get(ctx, metaURL, acceptXML)
	if err != nil {
		metaErr = err
	} else {
		switch status {
		case http.StatusOK:
			meta, perr := ParseMetadata(body)
			if perr != nil {
				metaErr = perr
			} else {
				versions = append(versions, VersionsFromMetadata(meta)...)
			}
		case http.StatusNotFound:
			// попробуем listing ниже
		case http.StatusUnauthorized, http.StatusForbidden:
			metaErr = fmt.Errorf("%s: HTTP %d", metaURL, status)
		default:
			metaErr = fmt.Errorf("%s: HTTP %d", metaURL, status)
		}
	}

	listVersions, listErr := c.versionsFromListing(ctx, repo, gav)
	if listErr == nil {
		versions = append(versions, listVersions...)
	}

	if latest := PickLatest(versions, c.IncludePreRelease); latest != "" {
		return latest, nil
	}

	// HTML/JSON Index пуст или без релизов — Artifactory API
	// (maven-metadata.xml иногда устаревший относительно Index / search API).
	if apiVersions, apiErr := c.versionsFromArtifactoryAPI(ctx, repo, gav); apiErr == nil {
		versions = append(versions, apiVersions...)
		if latest := PickLatest(versions, c.IncludePreRelease); latest != "" {
			return latest, nil
		}
	} else if listErr == nil {
		listErr = apiErr
	}

	if metaErr != nil {
		return "", metaErr
	}
	if listErr != nil {
		return "", listErr
	}
	return "", nil
}

// versionsFromListing читает каталог артефакта: HTML Index и/или JSON.
// Важно: Accept text/html — если просить application/json первым, Artifactory 7
// часто отдаёт «folder info» без children вместо Index of с версиями.
func (c *Client) versionsFromListing(ctx context.Context, repo string, gav gradle.GAV) ([]string, error) {
	dirURL := ArtifactDirURL(repo, gav.Group, gav.Artifact)
	body, status, err := c.get(ctx, dirURL, acceptHTML)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, nil
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d", dirURL, status)
	}
	if vers := VersionsFromArtifactoryStorageJSON(body); len(vers) > 0 {
		return vers, nil
	}
	return VersionsFromHTMLListing(string(body)), nil
}

// versionsFromArtifactoryAPI — один запрос /api/search/latestVersion (text/plain).
func (c *Client) versionsFromArtifactoryAPI(ctx context.Context, repo string, gav gradle.GAV) ([]string, error) {
	u, ok := ArtifactoryLatestVersionURL(repo, gav.Group, gav.Artifact)
	if !ok {
		return nil, nil
	}
	body, status, err := c.get(ctx, u, "text/plain, */*;q=0.1")
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, nil
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d", u, status)
	}
	v := VersionFromArtifactoryLatestVersionBody(body)
	if v == "" {
		return nil, nil
	}
	if !c.IncludePreRelease && !IsReleaseVersion(v) {
		c.debugf("  latestVersion=%q ignored (pre-release)", v)
		return nil, nil
	}
	c.debugf("  latestVersion hit: %s", v)
	return []string{v}, nil
}

const (
	acceptHTML = "text/html,application/xhtml+xml;q=0.9,*/*;q=0.8"
	acceptXML  = "application/xml,text/xml;q=0.9,*/*;q=0.8"
)

// get выполняет HTTP GET с optional Basic Auth и лимитом тела 16 MiB.
func (c *Client) get(ctx context.Context, url, accept string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	req.Header.Set("User-Agent", "depscout/"+UserAgentVersion)
	if c.User != "" {
		req.SetBasicAuth(c.User, c.Password)
	}
	start := time.Now()
	resp, err := c.HTTP.Do(req)
	if err != nil {
		c.debugf("HTTP GET %s → err=%v (%s)", url, err, time.Since(start).Round(time.Millisecond))
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		c.debugf("HTTP GET %s → %d read-err=%v (%s)", url, resp.StatusCode, err, time.Since(start).Round(time.Millisecond))
		return nil, resp.StatusCode, err
	}
	c.debugf("HTTP GET %s → %d (%s, %d bytes)", url, resp.StatusCode, time.Since(start).Round(time.Millisecond), len(data))
	return data, resp.StatusCode, nil
}

// UserAgentVersion — в HTTP User-Agent; bump-version синхронизирует с version.go.
var UserAgentVersion = "0.1.0"

// ResolveMany параллельно резолвит список GAV (без fallback по current).
func (c *Client) ResolveMany(ctx context.Context, gavs []gradle.GAV) []ResolveResult {
	return c.ResolveManyWithCurrent(ctx, gavs, nil)
}

// GetBytes — публичная обёртка над get (для wrapper.Resolver).
func (c *Client) GetBytes(ctx context.Context, url string) ([]byte, int, error) {
	return c.get(ctx, url, acceptHTML)
}
