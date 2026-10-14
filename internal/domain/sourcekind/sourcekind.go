// Package sourcekind registers the source types Semlia can connect. Each kind
// states how its metadata arrives, which uploaded artifact kinds it accepts and
// which execution dialect, if any, can run published plans against it.
package sourcekind

// Ingestion modes.
const (
	// Live sources are reached over the network with a stored credential.
	Live = "live"
	// Artifact sources are uploaded files pinned in an artifact set.
	Artifact = "artifact"
)

// Execution dialects understood by the plan compiler.
const (
	DialectPostgreSQL = "postgresql"
	DialectSQLite     = "sqlite"
)

type Kind struct {
	// Name is source_connections.source_kind.
	Name string
	// AdapterKind is source_connections.adapter_kind and the snapshot adapter.
	AdapterKind    string
	AdapterVersion string
	Ingestion      string
	// ArtifactKinds lists the uploaded artifact kinds an artifact source accepts.
	ArtifactKinds []string
	// Dialect is empty when published plans cannot execute on the source.
	Dialect string
}

var registry = []Kind{
	{Name: "postgresql", AdapterKind: "postgresql_catalog", AdapterVersion: "1.0.0", Ingestion: Live, Dialect: DialectPostgreSQL},
	{Name: "sqlite", AdapterKind: "sqlite_catalog", AdapterVersion: "1.0.0", Ingestion: Artifact, ArtifactKinds: []string{"sqlite"}, Dialect: DialectSQLite},
	{Name: "file", AdapterKind: "file_catalog", AdapterVersion: "1.0.0", Ingestion: Artifact, ArtifactKinds: []string{"csv", "xlsx", "markdown"}},
	{Name: "sql_bundle", AdapterKind: "postgresql_sql", AdapterVersion: "1.0.0", Ingestion: Artifact, ArtifactKinds: []string{"sql"}},
	{Name: "dbt_bundle", AdapterKind: "dbt", AdapterVersion: "1.0.0", Ingestion: Artifact, ArtifactKinds: []string{"dbt_manifest", "dbt_catalog"}},
}

// Lookup returns the registered kind with the given source_kind name.
func Lookup(name string) (Kind, bool) {
	for _, kind := range registry {
		if kind.Name == name {
			return kind, true
		}
	}
	return Kind{}, false
}

// ByAdapter returns the registered kind whose catalog adapter is adapterKind.
func ByAdapter(adapterKind string) (Kind, bool) {
	for _, kind := range registry {
		if kind.AdapterKind == adapterKind {
			return kind, true
		}
	}
	return Kind{}, false
}

// AdapterKind maps a source kind to its adapter kind, or "" when unknown.
func AdapterKind(name string) string {
	kind, _ := Lookup(name)
	return kind.AdapterKind
}

// Accepts reports whether an artifact source of this kind may contain artifactKind.
func Accepts(name, artifactKind string) bool {
	kind, ok := Lookup(name)
	if !ok {
		return false
	}
	for _, accepted := range kind.ArtifactKinds {
		if accepted == artifactKind {
			return true
		}
	}
	return false
}

// IsLive reports whether the source kind is reached over a live connection.
func IsLive(name string) bool {
	kind, ok := Lookup(name)
	return ok && kind.Ingestion == Live
}

// ExecutableAdapters lists the adapter kinds whose datasets published plans may query.
func ExecutableAdapters() []string {
	result := []string{}
	for _, kind := range registry {
		if kind.Dialect != "" {
			result = append(result, kind.AdapterKind)
		}
	}
	return result
}

// DialectForAdapter returns the execution dialect of an adapter kind, or "".
func DialectForAdapter(adapterKind string) string {
	kind, _ := ByAdapter(adapterKind)
	return kind.Dialect
}
