package maven

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
)

// Metadata — урезанная модель maven-metadata.xml для encoding/xml.
type Metadata struct {
	XMLName    xml.Name `xml:"metadata"`
	GroupID    string   `xml:"groupId"`
	ArtifactID string   `xml:"artifactId"`
	Versioning struct {
		Latest   string   `xml:"latest"`
		Release  string   `xml:"release"`
		Versions []string `xml:"versions>version"`
	} `xml:"versioning"`
}

// ParseMetadata разбирает XML maven-metadata.xml.
// Отклоняет пустое тело и HTML-ответы (часто login/error page с HTTP 200).
func ParseMetadata(data []byte) (*Metadata, error) {
	data = bytes.TrimSpace(data)
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF}) // UTF-8 BOM
	if len(data) == 0 {
		return nil, fmt.Errorf("parse maven-metadata: empty body")
	}
	head := data
	if len(head) > 64 {
		head = head[:64]
	}
	if bytes.HasPrefix(data, []byte("<!")) || bytes.Contains(head, []byte("<html")) {
		return nil, fmt.Errorf("parse maven-metadata: HTML response, not XML")
	}
	var m Metadata
	if err := xml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse maven-metadata: %w", err)
	}
	return &m, nil
}

// VersionsFromMetadata собирает все строки версий из metadata (versions + release + latest).
func VersionsFromMetadata(m *Metadata) []string {
	if m == nil {
		return nil
	}
	var out []string
	out = append(out, m.Versioning.Versions...)
	if m.Versioning.Release != "" {
		out = append(out, m.Versioning.Release)
	}
	if m.Versioning.Latest != "" {
		out = append(out, m.Versioning.Latest)
	}
	return out
}

// MetadataURL строит URL {repo}/{group.path}/{artifact}/maven-metadata.xml.
// Точки в groupId заменяются на '/' (стандартный Maven layout).
func MetadataURL(repo, group, artifact string) string {
	repo = strings.TrimRight(repo, "/")
	groupPath := strings.ReplaceAll(group, ".", "/")
	return fmt.Sprintf("%s/%s/%s/maven-metadata.xml", repo, groupPath, artifact)
}
