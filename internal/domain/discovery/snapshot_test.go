package discovery_test

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/iiwish/semlia/internal/domain/discovery"
)

func TestCoverageProofNormalizationAndCompatibility(t *testing.T) {
	unknown := discovery.Snapshot{}
	if err := unknown.CanonicalizeCoverage(); err != nil {
		t.Fatal(err)
	}
	if quality, status := unknown.CoverageState(); quality != "unverifiable" || status != "partial" {
		t.Fatalf("unknown=%s/%s", quality, status)
	}
	snapshot := discovery.Snapshot{AdapterKind: "fixture", AdapterVersion: "1.0.0"}
	snapshot.DeclareCoverage("b", "b.sql")
	snapshot.DeclareCoverage("a", "a.sql")
	snapshot.Coverage[0].DiagnosticCodes = []string{"WARNING_Z", "WARNING_A", "WARNING_A"}
	if err := snapshot.CanonicalizeCoverage(); err != nil {
		t.Fatal(err)
	}
	if snapshot.Coverage[0].Key != "a" || strings.Join(snapshot.Coverage[1].DiagnosticCodes, ",") != "WARNING_A,WARNING_Z" {
		t.Fatalf("coverage=%+v", snapshot.Coverage)
	}
	clone := snapshot.Clone()
	clone.Coverage[1].DiagnosticCodes[0] = "MUTATED"
	if snapshot.Coverage[1].DiagnosticCodes[0] != "WARNING_A" {
		t.Fatal("clone mutates caller")
	}
	before := snapshot.ScopeDigest()
	headBefore := discovery.CoverageSelectorDigest(snapshot.Coverage[0])
	snapshot.Coverage[0].ConfigDigest = discovery.SnapshotDigest("other config")
	if snapshot.ScopeDigest() == before {
		t.Fatal("source config is absent from scope digest")
	}
	if discovery.CoverageSelectorDigest(snapshot.Coverage[0]) != headBefore {
		t.Fatal("config change must invalidate the existing selector's freshness")
	}
	wire, _ := json.Marshal(snapshot.Coverage)
	if strings.Contains(string(wire), "sha256:") {
		t.Fatal("internal config digest leaked onto wire")
	}
	snapshot.Coverage[0].EnumerationComplete = false
	if err := snapshot.CanonicalizeCoverage(); err == nil {
		t.Fatal("unproven complete unit accepted")
	}
}

func TestSnapshotDiagnosticsDoNotExposeRawDetails(t *testing.T) {
	diagnostic := discovery.SafeSnapshotDiagnostic(discovery.Finding{Code: "invalid secret", Severity: "error", Locator: "postgres://password@host", Details: map[string]any{"password": "secret"}}, "catalog", 1)
	wire, _ := json.Marshal(diagnostic)
	if diagnostic.Code != "DISCOVERY_FAILED" || diagnostic.Severity != "blocker" || strings.Contains(string(wire), "secret") || strings.Contains(string(wire), "password") {
		t.Fatalf("unsafe diagnostic=%s", wire)
	}
}

func TestSnapshotDiagnosticsPreserveScopeAndMaximumSeverity(t *testing.T) {
	coverage := []discovery.CoverageUnit{{Key: "a", DiagnosticCodes: []string{"SHARED_CODE"}}, {Key: "b", DiagnosticCodes: []string{"SHARED_CODE", "FALLBACK_CODE"}}}
	findings := []discovery.Finding{
		{CoverageKey: "a", Code: "SHARED_CODE", Severity: "info"},
		{CoverageKey: "b", Code: "SHARED_CODE", Severity: "warning"},
		{CoverageKey: "b", Code: "SHARED_CODE", Severity: "error"},
		{CoverageKey: "b", Code: "SHARED_CODE", Severity: "info"},
	}
	want := []discovery.SnapshotDiagnostic{
		{Ordinal: 1, CoverageKey: "a", Code: "SHARED_CODE", Severity: "info", Message: "SHARED CODE"},
		{Ordinal: 2, CoverageKey: "b", Code: "FALLBACK_CODE", Severity: "warning", Message: "FALLBACK CODE"},
		{Ordinal: 3, CoverageKey: "b", Code: "SHARED_CODE", Severity: "blocker", Message: "SHARED CODE"},
	}
	for iteration := 0; iteration < 2; iteration++ {
		if got := discovery.SnapshotDiagnostics(findings, coverage); !reflect.DeepEqual(got, want) {
			t.Fatalf("diagnostics=%+v want=%+v", got, want)
		}
		slices.Reverse(findings)
	}
	global := discovery.SnapshotDiagnostics([]discovery.Finding{{Code: "GLOBAL_FAILURE", Severity: "info", Terminal: true}}, coverage)
	blockers := 0
	for _, diagnostic := range global {
		if diagnostic.Code == "GLOBAL_FAILURE" && diagnostic.Severity == "blocker" {
			blockers++
		}
	}
	if blockers != 2 {
		t.Fatalf("unscoped terminal failure covered %d units", blockers)
	}
}

func TestCoverageFindingsBindOnlyToDeclaredUnits(t *testing.T) {
	snapshot := discovery.Snapshot{Findings: []discovery.Finding{{CoverageKey: "b", Code: "SCOPED_FAILURE", Severity: "error"}}}
	snapshot.DeclareCoverage("a", "a.sql")
	snapshot.DeclareCoverage("b", "b.sql")
	if snapshot.Coverage[0].Status != "complete" || snapshot.Coverage[1].Status != "partial" {
		t.Fatalf("coverage=%+v", snapshot.Coverage)
	}
	snapshot.Findings[0].CoverageKey = "undeclared"
	if err := snapshot.CanonicalizeCoverage(); err == nil {
		t.Fatal("unknown diagnostic scope accepted")
	}
}

func TestPathCoverageKeyIsBoundedStableAndNamespaced(t *testing.T) {
	short := strings.Repeat("x", 252)
	if got := discovery.PathCoverageKey("sql", short); got != "sql:"+short {
		t.Fatalf("short readable key changed: %s", got)
	}
	long := strings.Repeat("x", 1024)
	key := discovery.PathCoverageKey("sql", long)
	if len(key) > 256 || key != discovery.PathCoverageKey("sql", long) || key == discovery.PathCoverageKey("sql", long[:1023]+"y") || key == discovery.PathCoverageKey("file", long) {
		t.Fatalf("unstable, unbounded or colliding key=%s", key)
	}
	if key == discovery.PathCoverageKey("sql", strings.TrimPrefix(key, "sql#")) {
		t.Fatal("digest namespace collides with a literal short path")
	}
}

func TestSnapshotCodeStorageRejectsBytesAboveUploadBoundary(t *testing.T) {
	snapshot := discovery.Snapshot{CodeArtifacts: []discovery.CodeArtifact{{Path: "schema.sql", Language: "sql", Content: make([]byte, (50<<20)+1)}}}
	snapshot.DeclareCoverage("sql:schema.sql", "schema.sql")
	if err := snapshot.CanonicalizeCoverage(); err == nil {
		t.Fatal("code retention accepted bytes beyond the existing internal staging boundary")
	}
}
