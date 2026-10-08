package maven

import (
	"errors"
	"strings"
	"testing"
)

func TestParseHostRewrites(t *testing.T) {
	m, err := ParseHostRewrites("old.example.com=new.example.net")
	if err != nil {
		t.Fatal(err)
	}
	if m["old.example.com"] != "new.example.net" {
		t.Fatalf("%v", m)
	}
	m, err = ParseHostRewrites(" https://a.com/=http://b.com/ , ,c.com=d.com ")
	if err != nil {
		t.Fatal(err)
	}
	if m["a.com"] != "b.com" || m["c.com"] != "d.com" {
		t.Fatalf("%v", m)
	}
	if _, err := ParseHostRewrites("bad"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := ParseHostRewrites("=x"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := ParseHostRewrites("x="); err == nil {
		t.Fatal("expected error")
	}
	if _, err := ParseHostRewrites("https://=host.com"); err == nil {
		t.Fatal("empty from host")
	}
	empty, err := ParseHostRewrites("")
	if err != nil || len(empty) != 0 {
		t.Fatal(empty, err)
	}
}

func TestHostRewritesFromEnv(t *testing.T) {
	t.Setenv("DEPSCOUT_REWRITE_HOST", "a.com=b.com")
	m, err := HostRewritesFromEnv()
	if err != nil || m["a.com"] != "b.com" {
		t.Fatalf("%v %v", m, err)
	}
}

func TestRewriteURLHost(t *testing.T) {
	rewrites := map[string]string{"old.example.com": "new.example.net"}
	got := RewriteURLHost("https://old.example.com/artifactory/public", rewrites)
	want := "https://new.example.net/artifactory/public"
	if got != want {
		t.Fatalf("got %s", got)
	}
	if RewriteURLHost("https://other.com/x", rewrites) != "https://other.com/x" {
		t.Fatal("unchanged")
	}
	if RewriteURLHost("", rewrites) != "" {
		t.Fatal("empty")
	}
	if RewriteURLHost("x", nil) != "x" {
		t.Fatal("nil map")
	}
	// unparseable / no host → string replace fallback
	got = RewriteURLHost("old.example.com/path", rewrites)
	if got != "new.example.net/path" {
		t.Fatalf("fallback %s", got)
	}
	got = RewriteURLHost("nothing-to-replace", rewrites)
	if got != "nothing-to-replace" {
		t.Fatal(got)
	}
}

func TestRewriteURLsAndMerge(t *testing.T) {
	in := []string{"https://old.example.com/r/", "https://keep.com/r"}
	out := RewriteURLs(in, nil)
	if len(out) != 2 || out[0] != in[0] {
		t.Fatal(out)
	}
	out = RewriteURLs(in, map[string]string{"old.example.com": "new.example.net"})
	if out[0] != "https://new.example.net/r" {
		t.Fatal(out)
	}
	merged := MergeRewrites(map[string]string{"a": "1", "b": "2"}, map[string]string{"b": "9", "c": "3"})
	if merged["a"] != "1" || merged["b"] != "9" || merged["c"] != "3" {
		t.Fatal(merged)
	}
}

func TestShortError(t *testing.T) {
	if ShortError(nil) != "" {
		t.Fatal("nil")
	}
	cases := []struct {
		err  string
		want string
	}{
		{"i/o timeout", "timeout"},
		{"context deadline exceeded", "timeout"},
		{"no such host", "DNS"},
		{"server misbehaving", "DNS"},
		{"connection refused", "connection refused"},
		{"certificate unknown", "TLS"},
		{"HTTP 401", "auth"},
		{"HTTP 403", "auth"},
		{"short", "short"},
	}
	for _, tc := range cases {
		got := ShortError(errors.New(tc.err))
		if !strings.Contains(got, tc.want) && got != tc.want {
			t.Fatalf("%q -> %q", tc.err, got)
		}
	}
	long := strings.Repeat("x", 120)
	got := ShortError(errors.New(long))
	if !strings.HasSuffix(got, "...") || len(got) > 100 {
		t.Fatalf("%q", got)
	}
}

func TestVersionsFromHTMLListing(t *testing.T) {
	html := `
<a href="../">../</a>
<a href="4.5.0/">4.5.0/</a>
<a href="4.6.0/">4.6.0/</a>
<a href="4.6.0-SNAPSHOT/">4.6.0-SNAPSHOT/</a>
<a href="maven-metadata.xml">maven-metadata.xml</a>
`
	vers := VersionsFromHTMLListing(html)
	latest := PickLatest(vers, false)
	if latest != "4.6.0" {
		t.Fatalf("got %s from %v", latest, vers)
	}
}
