package maven

import "testing"

func TestParseMetadata(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<metadata>
  <groupId>com.example</groupId>
  <artifactId>lib</artifactId>
  <versioning>
    <latest>1.2.0-SNAPSHOT</latest>
    <release>1.1.0</release>
    <versions>
      <version>1.0.0</version>
      <version>1.1.0</version>
      <version>1.2.0</version>
      <version>1.2.0-SNAPSHOT</version>
    </versions>
  </versioning>
</metadata>`
	m, err := ParseMetadata([]byte(xml))
	if err != nil {
		t.Fatal(err)
	}
	if m.GroupID != "com.example" || m.ArtifactID != "lib" {
		t.Fatalf("%+v", m)
	}
	if PickLatest(VersionsFromMetadata(m), false) != "1.2.0" {
		t.Fatalf("want 1.2.0, got %s", PickLatest(VersionsFromMetadata(m), false))
	}
}

func TestParseMetadataBOM(t *testing.T) {
	xml := append([]byte{0xEF, 0xBB, 0xBF}, []byte(`<metadata><versioning><versions><version>1.0.0</version></versions></versioning></metadata>`)...)
	m, err := ParseMetadata(xml)
	if err != nil {
		t.Fatal(err)
	}
	if PickLatest(VersionsFromMetadata(m), false) != "1.0.0" {
		t.Fatalf("got %s", PickLatest(VersionsFromMetadata(m), false))
	}
}

func TestParseMetadataHTMLRejected(t *testing.T) {
	_, err := ParseMetadata([]byte("<!DOCTYPE html><html></html>"))
	if err == nil {
		t.Fatal("expected error")
	}
	_, err = ParseMetadata([]byte("   "))
	if err == nil {
		t.Fatal("empty")
	}
	_, err = ParseMetadata([]byte("<notxml"))
	if err == nil {
		t.Fatal("bad xml")
	}
	longHTML := append([]byte("<html"), make([]byte, 100)...)
	copy(longHTML[5:], []byte(">zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"))
	_, err = ParseMetadata(longHTML)
	if err == nil {
		t.Fatal("html prefix")
	}
}

func TestVersionsFromMetadataNil(t *testing.T) {
	if VersionsFromMetadata(nil) != nil {
		t.Fatal("nil")
	}
	m := &Metadata{}
	m.Versioning.Release = "1.0"
	m.Versioning.Latest = "2.0"
	vs := VersionsFromMetadata(m)
	if len(vs) != 2 {
		t.Fatal(vs)
	}
}

func TestMetadataURL(t *testing.T) {
	u := MetadataURL("https://example.com/repo/", "com.google.protobuf", "com.google.protobuf.gradle.plugin")
	want := "https://example.com/repo/com/google/protobuf/com.google.protobuf.gradle.plugin/maven-metadata.xml"
	if u != want {
		t.Fatalf("got %s", u)
	}
}
