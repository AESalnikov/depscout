package maven

import "testing"

func TestParseRepoList(t *testing.T) {
	got := ParseRepoList(" https://a.example/r1/ ,https://b.example/r2,https://a.example/r1")
	if len(got) != 2 {
		t.Fatalf("%v", got)
	}
	if got[0] != "https://a.example/r1" || got[1] != "https://b.example/r2" {
		t.Fatalf("%v", got)
	}
	if len(ParseRepoList("")) != 0 {
		t.Fatal("empty")
	}
}

func TestMergeRepos(t *testing.T) {
	got := MergeRepos(
		[]string{"https://a.example/r1"},
		[]string{"https://b.example/r2", "https://a.example/r1"},
	)
	if len(got) != 2 {
		t.Fatalf("%v", got)
	}
}

func TestReposFromEnv(t *testing.T) {
	t.Setenv("DEPSCOUT_REPOS", "https://a.com/r,https://b.com/r")
	got := ReposFromEnv()
	if len(got) != 2 || got[0] != "https://a.com/r" {
		t.Fatal(got)
	}
}
