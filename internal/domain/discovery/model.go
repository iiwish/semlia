package discovery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"
)

var (
	ErrInvalidInput    = errors.New("invalid discovery input")
	ErrInvalidSnapshot = errors.New("invalid discovery snapshot")
	ErrUnsupported     = errors.New("unsupported discovery artifact")
)

type Input struct {
	Locator          string
	ExternalRevision string
	ObservedAt       time.Time
	Files            map[string][]byte
}

func (input Input) Validate(requiredFiles ...string) error {
	if input.Locator == "" || input.ObservedAt.IsZero() || len(input.Files) == 0 {
		return ErrInvalidInput
	}
	for _, name := range requiredFiles {
		if len(input.Files[name]) == 0 {
			return fmt.Errorf("%w: missing %s", ErrInvalidInput, name)
		}
	}
	for name := range input.Files {
		if name == "" || path.IsAbs(name) || strings.Contains(name, "\\") || strings.Contains("/"+name+"/", "/../") {
			return fmt.Errorf("%w: unsafe file name", ErrInvalidInput)
		}
	}
	return nil
}

func (input Input) ContentDigest() string {
	names := make([]string, 0, len(input.Files))
	for name := range input.Files {
		names = append(names, name)
	}
	sort.Strings(names)
	hash := sha256.New()
	for _, name := range names {
		_, _ = hash.Write([]byte(name))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write(input.Files[name])
		_, _ = hash.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

type Adapter interface {
	Kind() string
	Version() string
	Discover(context.Context, Input) (Snapshot, error)
}

type Snapshot struct {
	AdapterKind      string
	AdapterVersion   string
	ExternalRevision string
	Locator          string
	ContentDigest    string
	ObservedAt       time.Time
	Datasets         []Dataset
	CodeArtifacts    []CodeArtifact
	Lineage          []LineageEdge
	Keys             []KeyObservation
	Joins            []JoinObservation
	Findings         []Finding
	Coverage         []CoverageUnit
}

type Dataset struct {
	CoverageKey   string
	ExternalKey   string
	QualifiedName string
	Kind          string
	Locator       string
	Fingerprint   string
	Fields        []Field
}

type Field struct {
	ExternalKey string
	Name        string
	Ordinal     int
	DataType    string
	Nullable    bool
	Fingerprint string
}

type CodeArtifact struct {
	CoverageKey   string
	Content       []byte `json:"-"`
	Path          string
	BlobOID       string
	Language      string
	ContentDigest string
}

type LineageEdge struct {
	CoverageKey           string
	UpstreamExternalKey   string
	DownstreamExternalKey string
	Kind                  string
	CodePath              string
	Confidence            float64
}

type KeyObservation struct {
	DatasetExternalKey string
	ConstraintName     string
	FieldExternalKeys  []string
	Kind               string
}

type JoinObservation struct {
	ConstraintName         string
	FromDatasetExternalKey string
	FromFieldExternalKeys  []string
	ToDatasetExternalKey   string
	ToFieldExternalKeys    []string
}

type Finding struct {
	CoverageKey string `json:"coverageKey,omitempty"`
	Code        string
	Severity    string
	Locator     string
	Details     map[string]any
	Terminal    bool
}

func (snapshot *Snapshot) Canonicalize() error {
	if snapshot.AdapterKind == "" || snapshot.AdapterVersion == "" || snapshot.Locator == "" ||
		snapshot.ContentDigest == "" || snapshot.ObservedAt.IsZero() {
		return ErrInvalidSnapshot
	}
	datasetKeys := make(map[string]struct{}, len(snapshot.Datasets))
	for datasetIndex := range snapshot.Datasets {
		dataset := &snapshot.Datasets[datasetIndex]
		if dataset.ExternalKey == "" || dataset.QualifiedName == "" || dataset.Locator == "" || !validDatasetKind(dataset.Kind) {
			return ErrInvalidSnapshot
		}
		if _, exists := datasetKeys[dataset.ExternalKey]; exists {
			return fmt.Errorf("%w: duplicate dataset external key", ErrInvalidSnapshot)
		}
		datasetKeys[dataset.ExternalKey] = struct{}{}
		fieldKeys := make(map[string]struct{}, len(dataset.Fields))
		for fieldIndex := range dataset.Fields {
			field := &dataset.Fields[fieldIndex]
			if field.ExternalKey == "" || field.Name == "" || field.Ordinal < 1 || field.DataType == "" {
				return ErrInvalidSnapshot
			}
			if _, exists := fieldKeys[field.ExternalKey]; exists {
				return fmt.Errorf("%w: duplicate field external key", ErrInvalidSnapshot)
			}
			fieldKeys[field.ExternalKey] = struct{}{}
			field.Fingerprint = fingerprint(struct {
				ExternalKey string `json:"external_key"`
				Name        string `json:"name"`
				Ordinal     int    `json:"ordinal"`
				DataType    string `json:"data_type"`
				Nullable    bool   `json:"nullable"`
			}{field.ExternalKey, field.Name, field.Ordinal, field.DataType, field.Nullable})
		}
		sort.Slice(dataset.Fields, func(left, right int) bool {
			if dataset.Fields[left].Ordinal == dataset.Fields[right].Ordinal {
				return dataset.Fields[left].ExternalKey < dataset.Fields[right].ExternalKey
			}
			return dataset.Fields[left].Ordinal < dataset.Fields[right].Ordinal
		})
		dataset.Fingerprint = fingerprint(struct {
			ExternalKey   string  `json:"external_key"`
			QualifiedName string  `json:"qualified_name"`
			Kind          string  `json:"kind"`
			Fields        []Field `json:"fields"`
		}{dataset.ExternalKey, dataset.QualifiedName, dataset.Kind, dataset.Fields})
	}
	sort.Slice(snapshot.Datasets, func(left, right int) bool {
		return snapshot.Datasets[left].ExternalKey < snapshot.Datasets[right].ExternalKey
	})
	sort.Slice(snapshot.CodeArtifacts, func(left, right int) bool {
		leftKey := snapshot.CodeArtifacts[left].Path + "\x00" + snapshot.CodeArtifacts[left].ContentDigest
		rightKey := snapshot.CodeArtifacts[right].Path + "\x00" + snapshot.CodeArtifacts[right].ContentDigest
		return leftKey < rightKey
	})
	sort.Slice(snapshot.Lineage, func(left, right int) bool {
		leftKey := snapshot.Lineage[left].UpstreamExternalKey + "\x00" + snapshot.Lineage[left].DownstreamExternalKey +
			"\x00" + snapshot.Lineage[left].Kind + "\x00" + snapshot.Lineage[left].CodePath
		rightKey := snapshot.Lineage[right].UpstreamExternalKey + "\x00" + snapshot.Lineage[right].DownstreamExternalKey +
			"\x00" + snapshot.Lineage[right].Kind + "\x00" + snapshot.Lineage[right].CodePath
		return leftKey < rightKey
	})
	for index := range snapshot.Keys {
		key := &snapshot.Keys[index]
		if key.DatasetExternalKey == "" || key.ConstraintName == "" ||
			(key.Kind != "primary" && key.Kind != "unique") || len(key.FieldExternalKeys) == 0 {
			return ErrInvalidSnapshot
		}
		sort.Strings(key.FieldExternalKeys)
	}
	sort.Slice(snapshot.Keys, func(left, right int) bool {
		return snapshot.Keys[left].DatasetExternalKey+"\x00"+snapshot.Keys[left].ConstraintName <
			snapshot.Keys[right].DatasetExternalKey+"\x00"+snapshot.Keys[right].ConstraintName
	})
	for index := range snapshot.Joins {
		join := &snapshot.Joins[index]
		if join.ConstraintName == "" || join.FromDatasetExternalKey == "" || join.ToDatasetExternalKey == "" ||
			len(join.FromFieldExternalKeys) == 0 || len(join.ToFieldExternalKeys) == 0 ||
			len(join.FromFieldExternalKeys) != len(join.ToFieldExternalKeys) {
			return ErrInvalidSnapshot
		}
	}
	sort.Slice(snapshot.Joins, func(left, right int) bool {
		return snapshot.Joins[left].FromDatasetExternalKey+"\x00"+snapshot.Joins[left].ConstraintName <
			snapshot.Joins[right].FromDatasetExternalKey+"\x00"+snapshot.Joins[right].ConstraintName
	})
	sort.Slice(snapshot.Findings, func(left, right int) bool {
		leftKey := snapshot.Findings[left].Code + "\x00" + snapshot.Findings[left].Locator +
			"\x00" + fingerprint(snapshot.Findings[left])
		rightKey := snapshot.Findings[right].Code + "\x00" + snapshot.Findings[right].Locator +
			"\x00" + fingerprint(snapshot.Findings[right])
		return leftKey < rightKey
	})
	return snapshot.CanonicalizeCoverage()
}

func fingerprint(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func validDatasetKind(value string) bool {
	switch value {
	case "table", "view", "materialized_view", "model", "external":
		return true
	default:
		return false
	}
}
