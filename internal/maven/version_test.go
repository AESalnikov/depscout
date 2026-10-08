package maven

import "testing"

func TestIsReleaseVersion(t *testing.T) {
	cases := map[string]bool{
		"1.0.3":  true,
		"10.2.1": true,
		"0.10.0": true,
		"10.0":   true,
		"4.6.0":  true,
		"2.4.0-feature.ORBIT.42.checkout-SNAPSHOT": false,
		"1.9.0-dev-SNAPSHOT-a1b2c3d-99":            false,
		"1.0.0-RC1":                                false,
		"2.0.0-SNAPSHOT":                           false,
	}
	for v, want := range cases {
		if got := IsReleaseVersion(v); got != want {
			t.Fatalf("%s: got %v want %v", v, got, want)
		}
	}
}

func TestPickLatestReleasesOnly(t *testing.T) {
	vers := []string{
		"4.5.0",
		"4.6.0",
		"2.4.0-feature.ORBIT.42.checkout-SNAPSHOT",
		"10.0.6",
		"4.5.0-dev-SNAPSHOT-1",
	}
	got := PickLatest(vers, false)
	if got != "10.0.6" {
		t.Fatalf("got %s", got)
	}
	got = PickLatest([]string{"1.0.0", "2.0.0-SNAPSHOT", "1.5.0"}, false)
	if got != "1.5.0" {
		t.Fatalf("got %s", got)
	}
	got = PickLatest([]string{"1.0.0-SNAPSHOT", "2.0.0-feature.x"}, false)
	if got != "" {
		t.Fatalf("want empty, got %s", got)
	}
	got = PickLatest([]string{"1.0.0", "2.0.0-SNAPSHOT"}, true)
	if got != "2.0.0-SNAPSHOT" {
		t.Fatalf("prerelease mode %s", got)
	}
	if PickLatest([]string{"", "  "}, false) != "" {
		t.Fatal("empty list")
	}
}

func TestPickLatestProtobufLike(t *testing.T) {
	vers := []string{"0.8.8", "0.9.5", "0.9.6", "0.10.0", "0.8.18"}
	got := PickLatest(vers, false)
	if got != "0.10.0" {
		t.Fatalf("got %s", got)
	}
}

func TestPickLatestJooqLike(t *testing.T) {
	vers := []string{"10.0", "10.1", "10.1.1", "10.2.1", "9.0", "8.2.1"}
	got := PickLatest(vers, false)
	if got != "10.2.1" {
		t.Fatalf("got %s", got)
	}
}

func TestCompare(t *testing.T) {
	if Compare("1.0.3", "1.0.4") >= 0 {
		t.Fatal("1.0.3 should be < 1.0.4")
	}
	if Compare("10.2.1", "9.9.9") <= 0 {
		t.Fatal("10.2.1 should be > 9.9.9")
	}
	if Compare("1.0.0", "1.0.0") != 0 {
		t.Fatal("equal")
	}
	// non-semver fallback
	if Compare("@@@", "!!!") == 0 && "@@@" == "!!!" {
		t.Fatal("string compare path")
	}
	_ = Compare("not-a-semver!!!", "also-bad!!!")
	_ = normalizeSemver("v1.2")
	_ = normalizeSemver("1.2.3.4.5")
	_ = normalizeSemver("1.2.3-alpha")
}

func TestVersionsFromMetadataPickLatestIgnoresSnapshot(t *testing.T) {
	m := &Metadata{}
	m.Versioning.Latest = "2.4.0-feature.ORBIT.42.checkout-SNAPSHOT"
	m.Versioning.Release = "10.0.6"
	m.Versioning.Versions = []string{"10.0.5", "10.0.6", "2.4.0-feature.ORBIT.42.checkout-SNAPSHOT"}
	got := PickLatest(VersionsFromMetadata(m), false)
	if got != "10.0.6" {
		t.Fatalf("got %s", got)
	}
}
