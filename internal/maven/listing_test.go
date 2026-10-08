package maven

import "testing"

func TestArtifactDirURL(t *testing.T) {
	u := ArtifactDirURL("https://h/repo/", "com.ex", "lib")
	if u != "https://h/repo/com/ex/lib/" {
		t.Fatal(u)
	}
}

func TestVersionsFromHTMLListingCorners(t *testing.T) {
	html := `
<a href="../">../</a>
<a href="">/</a>
<a href="maven-metadata.xml">maven-metadata.xml</a>
<a href="file.xml/">file.xml/</a>
<a href="readme.md5/">readme.md5/</a>
<a href="x.sha1/">x.sha1/</a>
<a href="y.sha256/">y.sha256/</a>
<a href="z.sha512/">z.sha512/</a>
<a href="nodigits/">nodigits/</a>
<a href="1.0.0/">1.0.0/</a>
<a href="1.0.0/">1.0.0/</a>
<a href='2.0.0/'>2.0.0/</a>
<a href="3.0.0">3.0.0</a>
<a href="https://binary.example/artifactory/public/g/a/10.1.3/">10.1.3/</a>
<a href="/artifactory/public/g/a/10.0.6/">10.0.6/</a>
<a href="#top">#</a>
<a href="javascript:void(0)">js</a>
`
	vers := VersionsFromHTMLListing(html)
	got := PickLatest(vers, false)
	if got != "10.1.3" {
		t.Fatalf("got %s from %v", got, vers)
	}
	if looksLikeVersionDir("abc") {
		t.Fatal("no digit")
	}
}

func TestVersionNameFromHref(t *testing.T) {
	cases := map[string]string{
		"10.1.3/":                   "10.1.3",
		"10.1.3":                    "10.1.3",
		"https://h/repo/g/a/9.0.0/": "9.0.0",
		"/repo/g/a/8.0.0":           "8.0.0",
		"7.0.0?foo=1":               "7.0.0",
		"6.0.0#frag":                "6.0.0",
		"#":                         "",
		"javascript:x":              "",
		"JAVASCRIPT:alert(1)":       "",
		"data:text/html,hi":         "",
		"vbscript:msgbox":           "",
		"":                          "",
	}
	for in, want := range cases {
		if got := versionNameFromHref(in); got != want {
			t.Fatalf("%q: got %q want %q", in, got, want)
		}
	}
}
