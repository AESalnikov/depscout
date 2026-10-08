package report

// Kind — единый тип колонки KIND во всех источниках (Gradle + Maven).
type Kind string

const (
	KindProperty  Kind = "property"   // gradle.properties + ${prop}
	KindCatalog   Kind = "catalog"    // libs.versions.toml
	KindInline    Kind = "inline"     // "g:a:1.2.3" в build-скрипте
	KindPlugin    Kind = "plugin"     // plugins { }
	KindWrapper   Kind = "wrapper"    // gradle-wrapper.properties
	KindPom       Kind = "pom"        // pom.xml <properties>
	KindPomDep    Kind = "pom-dep"    // pom.xml dependency литерал
	KindPomPlugin Kind = "pom-plugin" // pom.xml plugin
)
