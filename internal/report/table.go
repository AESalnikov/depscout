// Package report форматирует результат depscout в текстовую таблицу (stdout)
// и считает summary для stderr.
package report

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// Status — исход проверки одной строки отчёта.
type Status string

const (
	StatusOK       Status = "OK"        // актуальная версия
	StatusOutdated Status = "OUTDATED"  // в репозитории есть новее
	StatusSkip     Status = "SKIP"      // нет маппинга на GAV
	StatusNotFound Status = "NOT_FOUND" // артефакт/релизы не найдены
	StatusError    Status = "ERROR"     // сеть/auth/парсинг
)

// Row — одна строка отчёта.
type Row struct {
	Kind    Kind   // единый Kind (см. kind.go)
	Name    string // имя свойства / plugin id / "gradle"
	Source  string // GAV или URL / "(unmapped)"
	Current string
	Latest  string
	Status  Status
}

// WriteTable печатает выровненную таблицу в w (обычно os.Stdout).
//
// text/tabwriter буферизует вывод до Flush: ошибки Fprintln/Fprintf на tw
// почти всегда nil, реальная ошибка записи в w приходит из Flush.
func WriteTable(w io.Writer, rows []Row) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "KIND\tNAME\tCOORDINATE / SOURCE\tCURRENT\tLATEST\tSTATUS")
	for _, r := range rows {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			r.Kind,
			r.Name,
			truncate(r.Source, 64),
			dash(r.Current),
			dash(r.Latest),
			r.Status,
		)
	}
	return tw.Flush()
}

// dash показывает "-" вместо пустой строки (удобнее читать таблицу).
func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// truncate обрезает длинные URL/ошибки, чтобы таблица не разъезжалась.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 3 {
		return s[:n]
	}
	return s[:n-3] + "..."
}

// CountOutdated считает строки со статусом OUTDATED (для --check).
func CountOutdated(rows []Row) int {
	return CountByStatus(rows, StatusOutdated)
}

// CountByStatus считает строки с указанным статусом.
func CountByStatus(rows []Row, status Status) int {
	n := 0
	for _, r := range rows {
		if r.Status == status {
			n++
		}
	}
	return n
}

// SummaryLine строит краткую сводку "ok=.. outdated=.. ..." для stderr.
func SummaryLine(rows []Row) string {
	var ok, outdated, skip, nf, errn int
	for _, r := range rows {
		switch r.Status {
		case StatusOK:
			ok++
		case StatusOutdated:
			outdated++
		case StatusSkip:
			skip++
		case StatusNotFound:
			nf++
		case StatusError:
			errn++
		}
	}
	parts := []string{
		fmt.Sprintf("ok=%d", ok),
		fmt.Sprintf("outdated=%d", outdated),
		fmt.Sprintf("skip=%d", skip),
		fmt.Sprintf("not_found=%d", nf),
	}
	if errn > 0 {
		parts = append(parts, fmt.Sprintf("error=%d", errn))
	}
	return strings.Join(parts, " ")
}
