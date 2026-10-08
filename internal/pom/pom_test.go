package pom

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AESalnikov/depscout/internal/report"
)

const samplePOM = `<?xml version="1.0" encoding="UTF-8"?>
<project>
  <modelVersion>4.0.0</modelVersion>
  <groupId>io.orbitcart</groupId>
  <artifactId>orbit-api</artifactId>
  <version>1.0.0-SNAPSHOT</version>
  <properties>
    <lib.version>1.0.0</lib.version>
    <compiler.version>3.11.0</compiler.version>
  </properties>
  <dependencyManagement>
    <dependencies>
      <dependency>
        <groupId>com.ex</groupId>
        <artifactId>bom</artifactId>
        <version>9.0.0</version>
        <type>pom</type>
        <scope>import</scope>
      </dependency>
    </dependencies>
  </dependencyManagement>
  <dependencies>
    <dependency>
      <groupId>com.ex</groupId>
      <artifactId>lib</artifactId>
      <version>${lib.version}</version>
    </dependency>
    <dependency>
      <groupId>com.ex</groupId>
      <artifactId>utils</artifactId>
      <version>2.0.0</version>
    </dependency>
    <dependency>
      <groupId>com.ex</groupId>
      <artifactId>dyn</artifactId>
      <version>1.+</version>
    </dependency>
    <dependency>
      <groupId>com.ex</groupId>
      <artifactId>nov</artifactId>
    </dependency>
  </dependencies>
  <build>
    <pluginManagement>
      <plugins>
        <plugin>
          <groupId>org.apache.maven.plugins</groupId>
          <artifactId>maven-surefire-plugin</artifactId>
          <version>3.0.0</version>
        </plugin>
      </plugins>
    </pluginManagement>
    <plugins>
      <plugin>
        <artifactId>maven-compiler-plugin</artifactId>
        <version>${compiler.version}</version>
      </plugin>
      <plugin>
        <groupId>org.apache.maven.plugins</groupId>
        <artifactId>maven-jar-plugin</artifactId>
        <version>3.3.0</version>
      </plugin>
    </plugins>
  </build>
</project>
`

func TestParseAndUpdate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pom.xml")
	if err := os.WriteFile(path, []byte(samplePOM), 0o644); err != nil {
		t.Fatal(err)
	}
	if !Exists(dir) {
		t.Fatal("Exists")
	}
	refs, err := Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) < 4 {
		t.Fatalf("got %d refs: %+v", len(refs), refs)
	}
	kinds := map[report.Kind]int{}
	for _, r := range refs {
		kinds[r.Kind]++
	}
	if kinds[KindProperty] < 1 || kinds[KindDep] < 1 || kinds[KindPlugin] < 1 {
		t.Fatalf("kinds=%v refs=%+v", kinds, refs)
	}

	if err := UpdateProperties(path, map[string]string{"lib.version": "1.2.0", "missing": "1"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "<lib.version>1.2.0</lib.version>") {
		t.Fatal(string(data))
	}
	if err := UpdateLiteralVersions(path, map[string]string{
		"com.ex:utils": "2.1.0",
		"org.apache.maven.plugins:maven-jar-plugin": "3.4.0",
		"bad": "1",
	}); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if !strings.Contains(string(data), "<version>2.1.0</version>") {
		t.Fatal(string(data))
	}
	if !strings.Contains(string(data), "<version>3.4.0</version>") {
		t.Fatal(string(data))
	}

	if err := UpdateProperties(path, nil); err != nil {
		t.Fatal(err)
	}
	if err := UpdateLiteralVersions(path, nil); err != nil {
		t.Fatal(err)
	}
}

func TestParseCorners(t *testing.T) {
	missing, err := Parse(filepath.Join(t.TempDir(), "nope.xml"))
	if err != nil || missing != nil {
		t.Fatal(err, missing)
	}
	dir := t.TempDir()
	bad := filepath.Join(dir, "pom.xml")
	if err := os.WriteFile(bad, []byte("<not-xml"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(bad); err == nil {
		t.Fatal("expected parse error")
	}
	if err := os.WriteFile(bad, []byte("<project></project>"), 0o644); err != nil {
		t.Fatal(err)
	}
	refs, err := Parse(bad)
	if err != nil || len(refs) != 0 {
		t.Fatal(err, refs)
	}

	// property key helpers / skip
	if propertyKey("${x}") != "x" || propertyKey("1.0") != "" {
		t.Fatal("propertyKey")
	}
	if !isSkippedVersion("1.+") || !isSkippedVersion("") || isSkippedVersion("1.0") {
		t.Fatal("skip")
	}
	props := parseProperties([]byte("<a>1</a><b>2</c>"))
	if props["a"] != "1" || props["b"] != "" {
		t.Fatal(props)
	}
}

func TestUpdateErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pom.xml")
	if err := os.WriteFile(path, []byte(samplePOM), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}
	_ = os.Chmod(dir, 0o555)
	defer func() {
		_ = os.Chmod(dir, 0o755)
		_ = os.Chmod(path, 0o644)
	}()
	if err := UpdateProperties(path, map[string]string{"lib.version": "9"}); err == nil {
		t.Fatal("expected write error")
	}
	if err := UpdateLiteralVersions(path, map[string]string{"com.ex:utils": "9"}); err == nil {
		t.Fatal("expected write error")
	}
}

func TestParseReadError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pom.xml")
	if err := os.WriteFile(path, []byte("<project/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(path, 0o644) }()
	if _, err := Parse(path); err == nil {
		t.Fatal("expected read error")
	}
}

func TestParseEdgeBranches(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pom.xml")
	content := `<?xml version="1.0"?>
<project>
  <properties>
    <ok.version>1.0</ok.version>
  </properties>
  <dependencies>
    <dependency>
      <groupId>com.ex</groupId>
      <artifactId>a</artifactId>
      <version>${}</version>
    </dependency>
    <dependency>
      <groupId>com.ex</groupId>
      <artifactId>b</artifactId>
      <version>${missing.version}</version>
    </dependency>
    <dependency>
      <groupId>com.ex</groupId>
      <artifactId>c</artifactId>
      <version>${ok.version}</version>
    </dependency>
    <dependency>
      <groupId>com.ex</groupId>
      <artifactId>c</artifactId>
      <version>${ok.version}</version>
    </dependency>
    <dependency>
      <groupId>com.ex</groupId>
      <artifactId>d</artifactId>
      <version>1.0.0</version>
    </dependency>
    <dependency>
      <groupId>com.ex</groupId>
      <artifactId>d</artifactId>
      <version>1.0.0</version>
    </dependency>
  </dependencies>
  <build>
    <plugins>
      <plugin>
        <artifactId>maven-compiler-plugin</artifactId>
        <version>${}</version>
      </plugin>
      <plugin>
        <artifactId>maven-surefire-plugin</artifactId>
        <version>${missing.version}</version>
      </plugin>
      <plugin>
        <artifactId>maven-jar-plugin</artifactId>
        <version>${ok.version}</version>
      </plugin>
      <plugin>
        <artifactId>maven-jar-plugin</artifactId>
        <version>${ok.version}</version>
      </plugin>
      <plugin>
        <groupId>org.apache.maven.plugins</groupId>
        <artifactId>maven-resources-plugin</artifactId>
        <version>1.+</version>
      </plugin>
      <plugin>
        <groupId>org.apache.maven.plugins</groupId>
        <artifactId>maven-source-plugin</artifactId>
        <version>3.0.0</version>
      </plugin>
      <plugin>
        <groupId>org.apache.maven.plugins</groupId>
        <artifactId>maven-source-plugin</artifactId>
        <version>3.0.0</version>
      </plugin>
      <plugin>
        <artifactId></artifactId>
        <version>1.0</version>
      </plugin>
    </plugins>
  </build>
</project>`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	refs, err := Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) < 2 {
		t.Fatalf("%+v", refs)
	}
}

func TestUpdateReadErrorsAndNoop(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pom.xml")
	if err := os.WriteFile(path, []byte(samplePOM), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpdateProperties(path, map[string]string{"no.such": "1"}); err != nil {
		t.Fatal(err)
	}
	if err := UpdateLiteralVersions(path, map[string]string{"com.ex:nope": "1", "bad": "1"}); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(path, 0o644) }()
	if err := UpdateProperties(path, map[string]string{"lib.version": "9"}); err == nil {
		t.Fatal("read props")
	}
	if err := UpdateLiteralVersions(path, map[string]string{"com.ex:utils": "9"}); err == nil {
		t.Fatal("read literals")
	}
}

func TestUpdateLiteralAltOrder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pom.xml")
	content := `<?xml version="1.0"?>
<project>
  <dependencies>
    <dependency>
      <artifactId>utils</artifactId>
      <groupId>com.ex</groupId>
      <version>1.0.0</version>
    </dependency>
  </dependencies>
</project>`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpdateLiteralVersions(path, map[string]string{"com.ex:utils": "2.0.0"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "<version>2.0.0</version>") {
		t.Fatal(string(data))
	}
}
