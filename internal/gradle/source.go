package gradle

import "github.com/AESalnikov/depscout/internal/report"

// Kind — алиас report.Kind (единый контракт KIND в отчёте).
type Kind = report.Kind

const (
	KindProperty = report.KindProperty
	KindCatalog  = report.KindCatalog
	KindInline   = report.KindInline
	KindPlugin   = report.KindPlugin
	KindWrapper  = report.KindWrapper
)
