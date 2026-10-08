// Command depscout — точка входа CLI.
// Логика вынесена в run(...), чтобы тестировать без os.Exit.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/AESalnikov/depscout/internal/maven"
	"github.com/AESalnikov/depscout/internal/report"
	"github.com/AESalnikov/depscout/internal/scout"
)

// osExit подменяется в тестах, чтобы main() не завершал процесс теста.
var osExit = os.Exit

func main() {
	osExit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run разбирает флаги, вызывает scout.Run и печатает отчёт.
// Возвращает код выхода процесса: 0 ok, 1 outdated(--check), 2 ошибка.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("depscout", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var (
		showVersion       = fs.Bool("version", false, "показать версию depscout и выйти")
		apply             = fs.Bool("apply", false, "записать обновления в файлы проекта")
		check             = fs.Bool("check", false, "код выхода 1, если есть устаревшие версии")
		includePreRelease = fs.Bool("include-prerelease", false, "учитывать alpha/beta/RC/SNAPSHOT")
		mapFile           = fs.String("map", "", "файл маппинга: prop=group:artifact (по строке)")
		catalogFile       = fs.String("catalog", "", "путь к libs.versions.toml (по умолчанию gradle/libs.versions.toml)")
		repos             = fs.String("repos", "", "Maven-репозитории через запятую (или DEPSCOUT_REPOS)")
		rewriteHost       = fs.String("rewrite-host", "", "подмена хоста в distributionUrl: old.host=new.host")
		skipDeps          = fs.Bool("skip-deps", false, "не проверять gradle.properties / inline")
		skipCatalog       = fs.Bool("skip-catalog", false, "не проверять Version Catalog")
		skipPlugins       = fs.Bool("skip-plugins", false, "не проверять блок plugins (Gradle)")
		skipWrapper       = fs.Bool("skip-wrapper", false, "не проверять Gradle Wrapper")
		skipPom           = fs.Bool("skip-pom", false, "не проверять Maven pom.xml")
		debug             = fs.Bool("debug", false, "лог HTTP/resolve на stderr (или DEPSCOUT_DEBUG=1)")
	)
	fs.Usage = func() {
		eprintln(stderr, "Использование: depscout [флаги] [путь-к-проекту]")
		eprintln(stderr, "")
		eprintln(stderr, "Gradle: catalog, properties, inline, plugins, wrapper.")
		eprintln(stderr, "Maven:  pom.xml (properties, dependencies, plugins).")
		eprintln(stderr, "")
		eprintln(stderr, "Только отчёт (ничего не меняет):")
		eprintln(stderr, "  export DEPSCOUT_REPOS=https://repo1.maven.org/maven2")
		eprintln(stderr, "  depscout /path/to/project")
		eprintln(stderr, "")
		eprintln(stderr, "Записать обновления:")
		eprintln(stderr, "  depscout --apply /path/to/project")
		eprintln(stderr, "")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *showVersion {
		_, _ = fmt.Fprintln(stdout, displayVersion())
		return 0
	}

	root := "."
	if fs.NArg() > 0 {
		root = fs.Arg(0)
	}

	repoList := maven.MergeRepos(maven.ReposFromEnv(), maven.ParseRepoList(*repos))

	rewrites, err := maven.ParseHostRewrites(*rewriteHost)
	if err != nil {
		eprintln(stderr, "depscout: %v", err)
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	res, err := scout.Run(ctx, scout.Options{
		Root:              root,
		Apply:             *apply,
		IncludePreRelease: *includePreRelease,
		Repos:             repoList,
		HostRewrites:      rewrites,
		MapFile:           *mapFile,
		CatalogFile:       *catalogFile,
		SkipDeps:          *skipDeps,
		SkipCatalog:       *skipCatalog,
		SkipPlugins:       *skipPlugins,
		SkipWrapper:       *skipWrapper,
		SkipPom:           *skipPom,
		Debug:             *debug || os.Getenv("DEPSCOUT_DEBUG") != "",
	})
	if err != nil {
		eprintln(stderr, "depscout: %v", err)
		if strings.Contains(err.Error(), "no Maven repositories") {
			eprintln(stderr, "  укажите --repos url1,url2  или  export DEPSCOUT_REPOS=url1,url2")
			eprintln(stderr, "  пример: --repos https://repo1.maven.org/maven2")
		}
		return 2
	}

	if len(res.Repos) > 0 {
		eprintln(stderr, "репозитории:")
		for _, r := range res.Repos {
			eprintln(stderr, "  %s", r)
		}
		eprintln(stderr, "")
	}

	if err := report.WriteTable(stdout, res.Rows); err != nil {
		eprintln(stderr, "depscout: %v", err)
		return 2
	}
	eprintln(stderr, "")
	eprintln(stderr, "%s", report.SummaryLine(res.Rows))
	if report.CountByStatus(res.Rows, report.StatusError) > 0 {
		eprintln(stderr, "подсказка: детали ERROR смотрите в колонке COORDINATE.")
		eprintln(stderr, "           проверьте URL --repos, сеть/VPN и DEPSCOUT_USER / DEPSCOUT_PASSWORD")
	}
	if *apply {
		eprintln(stderr, "обновления записаны")
	} else {
		eprintln(stderr, "dry-run (передайте --apply, чтобы записать изменения)")
	}

	if *check && report.CountOutdated(res.Rows) > 0 {
		return 1
	}
	return 0
}

// eprintln пишет диагностику в stderr (ошибки записи игнорируем — best effort).
func eprintln(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format+"\n", args...)
}
