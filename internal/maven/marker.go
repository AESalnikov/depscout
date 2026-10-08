package maven

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"strings"
	"sync"

	"github.com/AESalnikov/depscout/internal/gradle"
)

// markerPOM — урезанная модель Gradle plugin marker (packaging=pom).
// Обычно одна dependency на implementation jar; у marker metadata в Artifactory
// часто устаревший, у implementation — нет.

type markerPOM struct {
	XMLName xml.Name `xml:"project"`
	Deps    []struct {
		GroupID    string `xml:"groupId"`
		ArtifactID string `xml:"artifactId"`
		Version    string `xml:"version"`
	} `xml:"dependencies>dependency"`
}

// ArtifactPOMURL — URL конкретного POM: {repo}/{g}/{a}/{v}/{a}-{v}.pom
func ArtifactPOMURL(repo, group, artifact, version string) string {
	repo = strings.TrimRight(repo, "/")
	groupPath := strings.ReplaceAll(group, ".", "/")
	return fmt.Sprintf("%s/%s/%s/%s/%s-%s.pom", repo, groupPath, artifact, version, artifact, version)
}

// IsGradlePluginMarker — artifact вида *.gradle.plugin.
func IsGradlePluginMarker(artifact string) bool {
	return strings.HasSuffix(artifact, ".gradle.plugin")
}

// ImplementationGAVsFromMarkerPOM достаёт GAV зависимостей из marker POM.
func ImplementationGAVsFromMarkerPOM(data []byte) []gradle.GAV {
	data = bytes.TrimSpace(data)
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	if len(data) == 0 || bytes.HasPrefix(data, []byte("<!")) {
		return nil
	}
	var pom markerPOM
	if err := xml.Unmarshal(data, &pom); err != nil {
		return nil
	}
	var out []gradle.GAV
	seen := map[string]bool{}
	for _, d := range pom.Deps {
		g := strings.TrimSpace(d.GroupID)
		a := strings.TrimSpace(d.ArtifactID)
		if g == "" || a == "" || IsGradlePluginMarker(a) {
			continue
		}
		key := g + ":" + a
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, gradle.GAV{Group: g, Artifact: a})
	}
	return out
}

// ResolveLatestWithCurrent как ResolveLatest; при NOT_FOUND у plugin marker
// читает POM текущей версии и резолвит implementation GAV.
func (c *Client) ResolveLatestWithCurrent(ctx context.Context, gav gradle.GAV, current string) ResolveResult {
	res := c.ResolveLatest(ctx, gav)
	if res.Found || !IsGradlePluginMarker(gav.Artifact) {
		return res
	}
	current = strings.TrimSpace(current)
	if current == "" {
		c.debugf("%s not found (no current for marker fallback)", gav)
		return res
	}
	c.debugf("%s not found → marker-impl (current=%s)", gav, current)
	if via, ok := c.resolveViaMarkerImplementation(ctx, gav, current); ok {
		c.debugf("%s via marker-impl → %s", gav, via.Latest)
		return via
	}
	c.debugf("%s marker-impl miss", gav)
	return res
}

func (c *Client) resolveViaMarkerImplementation(ctx context.Context, marker gradle.GAV, current string) (ResolveResult, bool) {
	var impls []gradle.GAV
	for _, repo := range c.Repos {
		pomURL := ArtifactPOMURL(repo, marker.Group, marker.Artifact, current)
		body, status, err := c.get(ctx, pomURL, acceptXML)
		if err != nil || status != 200 {
			continue
		}
		impls = ImplementationGAVsFromMarkerPOM(body)
		if len(impls) > 0 {
			break
		}
	}
	if len(impls) == 0 {
		return ResolveResult{}, false
	}
	for _, impl := range impls {
		r := c.ResolveLatest(ctx, impl)
		if !r.Found {
			continue
		}
		// impl иногда публикуют раньше marker — берём версию, только если marker POM есть.
		for _, repo := range c.Repos {
			if !c.pomExists(ctx, repo, marker, r.Latest) {
				continue
			}
			return ResolveResult{GAV: marker, Latest: r.Latest, Repo: repo, Found: true}, true
		}
	}
	return ResolveResult{}, false
}

func (c *Client) pomExists(ctx context.Context, repo string, gav gradle.GAV, version string) bool {
	url := ArtifactPOMURL(repo, gav.Group, gav.Artifact, version)
	_, status, err := c.get(ctx, url, acceptXML)
	return err == nil && status == 200
}

// ResolveManyWithCurrent — как ResolveMany; currents[i] соответствует gavs[i].
func (c *Client) ResolveManyWithCurrent(ctx context.Context, gavs []gradle.GAV, currents []string) []ResolveResult {
	n := c.Concurrency
	if n <= 0 {
		n = 8
	}
	type job struct {
		i       int
		gav     gradle.GAV
		current string
	}
	jobs := make(chan job)
	results := make([]ResolveResult, len(gavs))
	var wg sync.WaitGroup
	for w := 0; w < n; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				results[j.i] = c.ResolveLatestWithCurrent(ctx, j.gav, j.current)
			}
		}()
	}
	for i, g := range gavs {
		cur := ""
		if i < len(currents) {
			cur = currents[i]
		}
		jobs <- job{i: i, gav: g, current: cur}
	}
	close(jobs)
	wg.Wait()
	return results
}
