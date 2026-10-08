package scout

import (
	"context"

	"github.com/AESalnikov/depscout/internal/gradle"
	"github.com/AESalnikov/depscout/internal/maven"
	"github.com/AESalnikov/depscout/internal/pom"
	"github.com/AESalnikov/depscout/internal/report"
)

// VersionRef — единая проверяемая координата из любого источника.
// Парсеры отдают свои типы; scout сводит их к VersionRef перед resolve/apply.
type VersionRef struct {
	Kind    report.Kind
	Name    string
	GAV     gradle.GAV
	Current string
	File    string // куда писать при --apply (пусто = только отчёт)

	// CatalogVersionKey — ключ [versions], если version.ref; иначе "".
	CatalogVersionKey string
	// PropertyKey — ключ gradle.properties / pom <properties>; иначе "".
	PropertyKey string
	// LiteralVersion — версия захардкожена на записи (catalog entry / pom dep).
	LiteralVersion bool
}

func fromCatalog(r gradle.CatalogRef, file string) VersionRef {
	name := r.Alias
	if r.VersionKey != "" {
		name = r.Alias + " (" + r.VersionKey + ")"
	}
	return VersionRef{
		Kind:              report.KindCatalog,
		Name:              name,
		GAV:               r.GAV,
		Current:           r.Current,
		File:              file,
		CatalogVersionKey: r.VersionKey,
		LiteralVersion:    r.LiteralOnEntry,
	}
}

func fromPlugin(p gradle.PluginRef) VersionRef {
	return VersionRef{
		Kind:    report.KindPlugin,
		Name:    p.ID,
		GAV:     p.GAV,
		Current: p.Version,
		File:    p.File,
	}
}

func fromInline(d gradle.InlineDependency) VersionRef {
	return VersionRef{
		Kind:           report.KindInline,
		Name:           d.GAV.String(),
		GAV:            d.GAV,
		Current:        d.Version,
		File:           d.File,
		LiteralVersion: true,
	}
}

func fromProperty(name string, gav gradle.GAV, current string) VersionRef {
	return VersionRef{
		Kind:        report.KindProperty,
		Name:        name,
		GAV:         gav,
		Current:     current,
		PropertyKey: name,
	}
}

func fromPom(r pom.Ref) VersionRef {
	return VersionRef{
		Kind:           r.Kind,
		Name:           r.Name,
		GAV:            r.GAV,
		Current:        r.Current,
		PropertyKey:    r.PropertyKey,
		LiteralVersion: r.LiteralVersion,
	}
}

func resolveVersionRefs(ctx context.Context, client *maven.Client, refs []VersionRef) []maven.ResolveResult {
	gavs := make([]gradle.GAV, len(refs))
	currents := make([]string, len(refs))
	for i, r := range refs {
		gavs[i] = r.GAV
		currents[i] = r.Current
	}
	return client.ResolveManyWithCurrent(ctx, gavs, currents)
}

func rowFromRef(ref VersionRef, res maven.ResolveResult, onOutdated func(latest string)) report.Row {
	row := report.Row{
		Kind:    ref.Kind,
		Name:    ref.Name,
		Source:  ref.GAV.String(),
		Current: ref.Current,
	}
	applyResolve(&row, res, ref.Current, onOutdated)
	return row
}
