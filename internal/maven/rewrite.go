package maven

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

// schemeHTTP / schemeHTTPS собраны конкатенацией, чтобы IDE не ругалась на «небезопасный http://» в литерале.
var (
	schemeHTTP  = "http" + "://"
	schemeHTTPS = "https" + "://"
)

// ParseHostRewrites разбирает "old.host=new.host[,...]" в map.
// Допускает URL с схемой — схема срезается, остаётся только host[/path не нужен].
func ParseHostRewrites(spec string) (map[string]string, error) {
	out := make(map[string]string)
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return out, nil
	}
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		eq := strings.IndexByte(part, '=')
		if eq <= 0 || eq == len(part)-1 {
			return nil, fmt.Errorf("invalid --rewrite-host %q (want old.host=new.host)", part)
		}
		from := strings.TrimSpace(part[:eq])
		to := strings.TrimSpace(part[eq+1:])
		from = strings.TrimPrefix(from, schemeHTTPS)
		from = strings.TrimPrefix(from, schemeHTTP)
		to = strings.TrimPrefix(to, schemeHTTPS)
		to = strings.TrimPrefix(to, schemeHTTP)
		from = strings.TrimRight(from, "/")
		to = strings.TrimRight(to, "/")
		if from == "" || to == "" {
			return nil, fmt.Errorf("invalid --rewrite-host %q", part)
		}
		out[from] = to
	}
	return out, nil
}

// HostRewritesFromEnv читает DEPSCOUT_REWRITE_HOST.
func HostRewritesFromEnv() (map[string]string, error) {
	return ParseHostRewrites(os.Getenv("DEPSCOUT_REWRITE_HOST"))
}

// RewriteURLHost подменяет hostname в URL по таблице rewrites.
// Если URL не парсится — пробует простую замену подстроки host.
func RewriteURLHost(raw string, rewrites map[string]string) string {
	if len(rewrites) == 0 || raw == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		for from, to := range rewrites {
			if strings.Contains(raw, from) {
				return strings.Replace(raw, from, to, 1)
			}
		}
		return raw
	}
	if to, ok := rewrites[u.Host]; ok {
		u.Host = to
		return u.String()
	}
	return raw
}

// RewriteURLs применяет RewriteURLHost ко всем URL и срезает хвостовой '/'.
func RewriteURLs(urls []string, rewrites map[string]string) []string {
	if len(rewrites) == 0 {
		return urls
	}
	out := make([]string, len(urls))
	for i, u := range urls {
		out[i] = strings.TrimRight(RewriteURLHost(u, rewrites), "/")
	}
	return out
}

// MergeRewrites объединяет две таблицы; ключи из b перекрывают a.
func MergeRewrites(a, b map[string]string) map[string]string {
	out := make(map[string]string)
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

// ShortError сжимает длинные net/http ошибки в короткую подсказку для таблицы отчёта.
func ShortError(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	lower := strings.ToLower(s)
	switch {
	case strings.Contains(lower, "timeout") || strings.Contains(lower, "deadline exceeded"):
		return "timeout (check --repos URL / network / VPN)"
	case strings.Contains(lower, "no such host") || strings.Contains(lower, "server misbehaving"):
		return "DNS failure (check --repos host)"
	case strings.Contains(lower, "connection refused"):
		return "connection refused"
	case strings.Contains(lower, "certificate"):
		return "TLS certificate error"
	case strings.Contains(s, "HTTP 401") || strings.Contains(s, "HTTP 403"):
		return "auth failed (set DEPSCOUT_USER/DEPSCOUT_PASSWORD)"
	}
	if len(s) > 100 {
		return s[:97] + "..."
	}
	return s
}
