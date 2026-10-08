package report

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, errors.New("write fail") }

func TestWriteTableAndHelpers(t *testing.T) {
	rows := []Row{
		{Kind: KindProperty, Name: "a", Source: "g:a", Current: "1", Latest: "2", Status: StatusOutdated},
		{Kind: KindPlugin, Name: "b", Source: "", Current: "", Latest: "", Status: StatusOK},
		{Kind: Kind("x"), Name: "y", Source: strings.Repeat("s", 80), Current: "1", Latest: "1", Status: StatusSkip},
		{Kind: Kind("x"), Name: "y", Source: "z", Current: "1", Latest: "", Status: StatusNotFound},
		{Kind: Kind("x"), Name: "y", Source: "z", Current: "1", Latest: "", Status: StatusError},
	}
	var buf bytes.Buffer
	if err := WriteTable(&buf, rows); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "OUTDATED") || !strings.Contains(out, "...") {
		t.Fatalf("%s", out)
	}
	if err := WriteTable(errWriter{}, rows); err == nil {
		t.Fatal("expected write error")
	}

	if dash("") != "-" || dash("x") != "x" {
		t.Fatal("dash")
	}
	if truncate("ab", 5) != "ab" {
		t.Fatal("short")
	}
	if truncate("abcdef", 3) != "abc" {
		t.Fatal("n<=3")
	}
	if truncate("abcdef", 5) != "ab..." {
		t.Fatal("ellipsis")
	}
	if CountOutdated(rows) != 1 {
		t.Fatal("outdated")
	}
	if CountByStatus(rows, StatusError) != 1 {
		t.Fatal("error count")
	}
	sum := SummaryLine(rows)
	if !strings.Contains(sum, "ok=1") || !strings.Contains(sum, "error=1") {
		t.Fatal(sum)
	}
	sum2 := SummaryLine([]Row{{Status: StatusOK}})
	if strings.Contains(sum2, "error=") {
		t.Fatal(sum2)
	}
}
