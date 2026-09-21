package discovery

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
)

const SnapshotPageBytes = 1 << 20
const MaxCodeBytes = 50 << 20

var diagnosticCode = regexp.MustCompile(`^[A-Z][A-Z0-9_]{2,63}$`)
var digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

type CoverageUnit struct {
	Key                 string   `json:"key"`
	Selector            string   `json:"selector"`
	ConfigDigest        string   `json:"-"`
	Status              string   `json:"status"`
	EnumerationComplete bool     `json:"enumerationComplete"`
	DiagnosticCodes     []string `json:"diagnosticCodes"`
}

type SourceSnapshot struct {
	ID               string         `json:"id"`
	SourceID         string         `json:"sourceId"`
	SourceRevisionID string         `json:"sourceRevisionId"`
	AdapterVersion   string         `json:"adapterVersion"`
	ScopeDigest      string         `json:"scopeDigest"`
	ContentDigest    string         `json:"contentDigest"`
	HistoryQuality   string         `json:"historyQuality"`
	CoverageStatus   string         `json:"coverageStatus"`
	Coverage         []CoverageUnit `json:"coverage"`
	MemberCount      int            `json:"memberCount"`
	DiagnosticCount  int            `json:"diagnosticCount"`
	CreatedAt        time.Time      `json:"createdAt"`
}

type SnapshotMember struct {
	Kind             string `json:"kind"`
	ObjectID         string `json:"objectId"`
	RevisionID       string `json:"revisionId"`
	Name             string `json:"name"`
	Locator          string `json:"locator"`
	ContentDigest    string `json:"contentDigest"`
	CoverageKey      string `json:"coverageKey"`
	ParentObjectID   string `json:"parentObjectId,omitempty"`
	ParentRevisionID string `json:"parentRevisionId,omitempty"`
	DatasetKind      string `json:"datasetKind,omitempty"`
	DataType         string `json:"dataType,omitempty"`
	Nullable         *bool  `json:"nullable,omitempty"`
	Ordinal          *int   `json:"ordinal,omitempty"`
}

type SnapshotDiagnostic struct {
	Ordinal     int    `json:"ordinal"`
	Code        string `json:"code"`
	Severity    string `json:"severity"`
	CoverageKey string `json:"coverageKey"`
	Locator     string `json:"locator,omitempty"`
	Message     string `json:"message"`
}

type SnapshotPage struct {
	Items      []SourceSnapshot `json:"items"`
	NextCursor *string          `json:"nextCursor"`
}
type SnapshotMemberPage struct {
	SnapshotID     string           `json:"snapshotId"`
	HistoryQuality string           `json:"historyQuality"`
	Items          []SnapshotMember `json:"items"`
	NextCursor     *string          `json:"nextCursor"`
}
type SnapshotDiagnosticPage struct {
	SnapshotID string               `json:"snapshotId"`
	Items      []SnapshotDiagnostic `json:"items"`
	NextCursor *string              `json:"nextCursor"`
}

func SnapshotDigest(value any) string { return fingerprint(value) }

func CoverageSelectorDigest(unit CoverageUnit) string {
	digest := sha256.Sum256([]byte(unit.Selector))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func PathCoverageKey(kind, path string) string {
	key := kind + ":" + path
	if len(key) <= 256 {
		return key
	}
	digest := sha256.Sum256([]byte(path))
	// A separate namespace cannot collide with a short literal path's key.
	return kind + "#sha256:" + hex.EncodeToString(digest[:])
}

func (snapshot Snapshot) Clone() Snapshot {
	snapshot.Datasets = slices.Clone(snapshot.Datasets)
	for i := range snapshot.Datasets {
		snapshot.Datasets[i].Fields = slices.Clone(snapshot.Datasets[i].Fields)
	}
	snapshot.CodeArtifacts = slices.Clone(snapshot.CodeArtifacts)
	for i := range snapshot.CodeArtifacts {
		snapshot.CodeArtifacts[i].Content = slices.Clone(snapshot.CodeArtifacts[i].Content)
	}
	snapshot.Lineage = slices.Clone(snapshot.Lineage)
	snapshot.Coverage = slices.Clone(snapshot.Coverage)
	for i := range snapshot.Coverage {
		snapshot.Coverage[i].DiagnosticCodes = slices.Clone(snapshot.Coverage[i].DiagnosticCodes)
	}
	snapshot.Findings = slices.Clone(snapshot.Findings)
	snapshot.Keys = slices.Clone(snapshot.Keys)
	for i := range snapshot.Keys {
		snapshot.Keys[i].FieldExternalKeys = slices.Clone(snapshot.Keys[i].FieldExternalKeys)
	}
	snapshot.Joins = slices.Clone(snapshot.Joins)
	for i := range snapshot.Joins {
		snapshot.Joins[i].FromFieldExternalKeys = slices.Clone(snapshot.Joins[i].FromFieldExternalKeys)
		snapshot.Joins[i].ToFieldExternalKeys = slices.Clone(snapshot.Joins[i].ToFieldExternalKeys)
	}
	return snapshot
}

// DeclareCoverage records only the adapter's declared input, never an inferred database-wide scope.
func (snapshot *Snapshot) DeclareCoverage(key, selector string) {
	status, complete := "complete", true
	codes := []string{}
	for _, finding := range snapshot.Findings {
		if finding.CoverageKey != "" && finding.CoverageKey != key {
			continue
		}
		codes = append(codes, finding.Code)
		if finding.Terminal {
			status, complete = "failed", false
		} else if status != "failed" && finding.Severity != "info" {
			status, complete = "partial", false
		}
	}
	snapshot.Coverage = append(snapshot.Coverage, CoverageUnit{Key: key, Selector: selector, ConfigDigest: fingerprint(struct{ Kind, Version string }{snapshot.AdapterKind, snapshot.AdapterVersion}), Status: status, EnumerationComplete: complete, DiagnosticCodes: codes})
	for index := range snapshot.Datasets {
		if snapshot.Datasets[index].CoverageKey == "" {
			snapshot.Datasets[index].CoverageKey = key
		}
	}
	for index := range snapshot.CodeArtifacts {
		if snapshot.CodeArtifacts[index].CoverageKey == "" {
			snapshot.CodeArtifacts[index].CoverageKey = key
		}
	}
	for index := range snapshot.Lineage {
		if snapshot.Lineage[index].CoverageKey == "" {
			snapshot.Lineage[index].CoverageKey = key
		}
	}
}

func (snapshot *Snapshot) CanonicalizeCoverage() error {
	if len(snapshot.Coverage) == 0 {
		// Compatibility callers cannot prove completeness merely by omitting a declaration.
		snapshot.DeclareCoverage("unknown", "unknown")
		snapshot.Coverage[0].Status, snapshot.Coverage[0].EnumerationComplete = "partial", false
		snapshot.Coverage[0].DiagnosticCodes = []string{"COVERAGE_UNDECLARED"}
	}
	if len(snapshot.Coverage) > 256 {
		return ErrInvalidSnapshot
	}
	encodedCoverage, _ := json.Marshal(snapshot.Coverage)
	if len(encodedCoverage) > 256<<10 {
		return ErrInvalidSnapshot
	}
	seen := map[string]bool{}
	for index := range snapshot.Coverage {
		unit := &snapshot.Coverage[index]
		if unit.Key == "" || len(unit.Key) > 256 || len(unit.Selector) > 1024 || seen[unit.Key] || !digestPattern.MatchString(unit.ConfigDigest) || (unit.Status != "complete" && unit.Status != "partial" && unit.Status != "failed") || (unit.Status == "complete" && !unit.EnumerationComplete) || len(unit.DiagnosticCodes) > 64 {
			return ErrInvalidSnapshot
		}
		seen[unit.Key] = true
		sort.Strings(unit.DiagnosticCodes)
		codes := make([]string, 0, len(unit.DiagnosticCodes))
		for _, code := range unit.DiagnosticCodes {
			if !diagnosticCode.MatchString(code) {
				return ErrInvalidSnapshot
			}
			if len(codes) == 0 || codes[len(codes)-1] != code {
				codes = append(codes, code)
			}
		}
		unit.DiagnosticCodes = codes
	}
	sort.Slice(snapshot.Coverage, func(i, j int) bool { return snapshot.Coverage[i].Key < snapshot.Coverage[j].Key })
	for index := range snapshot.Findings {
		finding := &snapshot.Findings[index]
		if finding.CoverageKey == "" && len(snapshot.Coverage) == 1 {
			finding.CoverageKey = snapshot.Coverage[0].Key
		}
		if finding.CoverageKey != "" && !seen[finding.CoverageKey] {
			return ErrInvalidSnapshot
		}
	}
	for _, dataset := range snapshot.Datasets {
		if !seen[dataset.CoverageKey] || len(dataset.QualifiedName) > 1024 || len(dataset.Locator) > 2048 {
			return ErrInvalidSnapshot
		}
		for _, field := range dataset.Fields {
			if len(field.Name) > 1024 || len(dataset.Locator)+len(field.Name)+1 > 2048 {
				return ErrInvalidSnapshot
			}
		}
	}
	for _, code := range snapshot.CodeArtifacts {
		if !seen[code.CoverageKey] || code.Path == "" || len(code.Path) > 1024 || code.Language == "" || len(code.Content) > MaxCodeBytes {
			return ErrInvalidSnapshot
		}
	}
	for _, edge := range snapshot.Lineage {
		if !seen[edge.CoverageKey] {
			return ErrInvalidSnapshot
		}
	}
	return nil
}

func (snapshot Snapshot) CoverageState() (quality, status string) {
	quality, status = "verified", "complete"
	allFailed := true
	for _, unit := range snapshot.Coverage {
		if unit.Key == "unknown" {
			quality = "unverifiable"
		}
		if unit.Status != "failed" {
			allFailed = false
		}
		if unit.Status != "complete" {
			status = "partial"
		}
	}
	if allFailed {
		status = "failed"
	}
	for _, code := range snapshot.CodeArtifacts {
		if len(code.Content) == 0 {
			quality = "unverifiable"
		}
	}
	return
}

func (snapshot Snapshot) ScopeDigest() string {
	type scope struct{ Key, Selector, ConfigDigest string }
	units := make([]scope, 0, len(snapshot.Coverage))
	for _, unit := range snapshot.Coverage {
		units = append(units, scope{unit.Key, unit.Selector, unit.ConfigDigest})
	}
	return fingerprint(units)
}

func SafeSnapshotDiagnostic(finding Finding, key string, ordinal int) SnapshotDiagnostic {
	severity := finding.Severity
	if severity == "error" || finding.Terminal {
		severity = "blocker"
	}
	if severity != "info" && severity != "warning" && severity != "blocker" {
		severity = "blocker"
	}
	code := finding.Code
	if !diagnosticCode.MatchString(code) {
		code = "DISCOVERY_FAILED"
	}
	// Raw adapter details and locators may contain secrets; only stable codes are published.
	return SnapshotDiagnostic{Ordinal: ordinal, Code: code, Severity: severity, CoverageKey: key, Message: strings.ReplaceAll(code, "_", " ")}
}

func SnapshotDiagnostics(findings []Finding, coverage []CoverageUnit) []SnapshotDiagnostic {
	type diagnosticKey struct{ coverage, code string }
	byKey := map[diagnosticKey]SnapshotDiagnostic{}
	severityRank := map[string]int{"info": 0, "warning": 1, "blocker": 2}
	for _, finding := range findings {
		for _, unit := range coverage {
			// Unscoped failures conservatively apply to all declared units.
			if finding.CoverageKey != "" && finding.CoverageKey != unit.Key {
				continue
			}
			diagnostic := SafeSnapshotDiagnostic(finding, unit.Key, 0)
			key := diagnosticKey{unit.Key, diagnostic.Code}
			current, exists := byKey[key]
			if !exists || severityRank[diagnostic.Severity] > severityRank[current.Severity] {
				byKey[key] = diagnostic
			}
		}
	}
	for _, unit := range coverage {
		for _, code := range unit.DiagnosticCodes {
			diagnostic := SafeSnapshotDiagnostic(Finding{Code: code, Severity: "warning"}, unit.Key, 0)
			key := diagnosticKey{unit.Key, diagnostic.Code}
			if _, exists := byKey[key]; !exists {
				byKey[key] = diagnostic
			}
		}
	}
	diagnostics := make([]SnapshotDiagnostic, 0, len(byKey))
	for _, diagnostic := range byKey {
		diagnostics = append(diagnostics, diagnostic)
	}
	sort.Slice(diagnostics, func(i, j int) bool {
		if diagnostics[i].CoverageKey != diagnostics[j].CoverageKey {
			return diagnostics[i].CoverageKey < diagnostics[j].CoverageKey
		}
		return diagnostics[i].Code < diagnostics[j].Code
	})
	for i := range diagnostics {
		diagnostics[i].Ordinal = i + 1
	}
	return diagnostics
}
