package maven

import (
	"testing"
)

func FuzzCompare(f *testing.F) {
	f.Add("1.0.0", "1.0.1")
	f.Add("2.4.1", "2.5.0")
	f.Add("10.0.6", "10.1.3")
	f.Add("", "1")
	f.Add("1.0.0-SNAPSHOT", "1.0.0")
	f.Fuzz(func(t *testing.T, a, b string) {
		_ = Compare(a, b)
		_ = IsReleaseVersion(a)
		_ = PickLatest([]string{a, b}, false)
		_ = PickLatest([]string{a, b}, true)
	})
}

func FuzzVersionsFromHTMLListing(f *testing.F) {
	f.Add(`<a href="1.2.3/">1.2.3/</a>`)
	f.Add("Index of /\n10.0.6/  01-Jan\n10.1.3/\n")
	f.Add(`{"children":[{"uri":"/1.0.0"}]}`)
	f.Add("")
	f.Fuzz(func(t *testing.T, html string) {
		_ = VersionsFromHTMLListing(html)
		_ = VersionsFromArtifactoryStorageJSON([]byte(html))
		_ = VersionFromArtifactoryLatestVersionBody([]byte(html))
	})
}

func FuzzParseMetadata(f *testing.F) {
	f.Add([]byte(`<metadata><versioning><versions><version>1.0</version></versions></versioning></metadata>`))
	f.Add([]byte(`<?xml version="1.0"?><metadata/>`))
	f.Add([]byte(``))
	f.Fuzz(func(t *testing.T, data []byte) {
		m, err := ParseMetadata(data)
		if err != nil {
			return
		}
		_ = VersionsFromMetadata(m)
	})
}

func FuzzImplementationGAVsFromMarkerPOM(f *testing.F) {
	f.Add([]byte(`<?xml version="1.0"?><project><dependencies>
  <dependency><groupId>a.b</groupId><artifactId>c</artifactId></dependency>
</dependencies></project>`))
	f.Add([]byte(`<!DOCTYPE html>`))
	f.Fuzz(func(t *testing.T, data []byte) {
		_ = ImplementationGAVsFromMarkerPOM(data)
	})
}
