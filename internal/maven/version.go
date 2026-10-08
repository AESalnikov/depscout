package maven

import (
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"
)

// releaseVersionRe — только «чистые» числовые релизы: 1, 1.2, 1.2.3, 1.2.3.4.
// Всё с суффиксом (-SNAPSHOT, -feature..., -RC) релизом не считается.
var releaseVersionRe = regexp.MustCompile(`^[0-9]+(\.[0-9]+)*$`)

// IsReleaseVersion сообщает, является ли строка строгим числовым релизом.
func IsReleaseVersion(v string) bool {
	v = strings.TrimSpace(v)
	return v != "" && releaseVersionRe.MatchString(v)
}

// Compare сравнивает две версии: -1 если a<b, 0 если равны, 1 если a>b.
// Сначала пытается semver (через Masterminds), иначе — обычное строковое сравнение.
func Compare(a, b string) int {
	sa, ea := semver.NewVersion(normalizeSemver(a))
	sb, eb := semver.NewVersion(normalizeSemver(b))
	if ea == nil && eb == nil {
		return sa.Compare(sb)
	}
	return strings.Compare(a, b)
}

// normalizeSemver дополняет версию до major.minor.patch, чтобы парсер semver не падал на "1.2".
func normalizeSemver(v string) string {
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, "-", 2)
	core := parts[0]
	segs := strings.Split(core, ".")
	for len(segs) < 3 {
		segs = append(segs, "0")
	}
	if len(segs) > 3 {
		segs = segs[:3]
	}
	out := strings.Join(segs, ".")
	if len(parts) == 2 {
		out += "-" + parts[1]
	}
	return out
}

// PickLatest выбирает максимальную версию из списка.
//
// Если includePreRelease=false — учитываются только IsReleaseVersion.
// Если подходящих нет — возвращает "" (намеренно без fallback на SNAPSHOT).
func PickLatest(versions []string, includePreRelease bool) string {
	var candidates []string
	for _, v := range versions {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if !includePreRelease && !IsReleaseVersion(v) {
			continue
		}
		candidates = append(candidates, v)
	}
	best := ""
	for _, v := range candidates {
		if best == "" || Compare(v, best) > 0 {
			best = v
		}
	}
	return best
}
