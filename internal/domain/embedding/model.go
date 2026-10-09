package embedding

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/iiwish/semlia/pkg/identity"
)

var (
	ErrInvalid       = errors.New("invalid embedding request")
	ErrNotConfigured = errors.New("embedding is not configured")
	// ErrNoPublishedRelease reports that the workspace has no published release,
	// so there is no released corpus for a new index to cover. It is deliberately
	// distinct from ErrNotConfigured: the capability is available, the work the
	// operator must do first (publish a release) lives in another surface.
	ErrNoPublishedRelease = errors.New("embedding requires a published release")
	ErrConflict           = errors.New("embedding state conflict")
	ErrNotFound           = errors.New("embedding index not found")
	ErrProvider           = errors.New("embedding provider unavailable")
	ErrLeaseLost          = errors.New("embedding lease lost")
)

// Status reasons. They are machine-readable so the client can tell an operator
// what to fix instead of reporting one opaque "unavailable" state.
const (
	// ReasonReady means the capability is configured and a rebuild can start.
	ReasonReady = ""
	// ReasonNotConfigured means pgvector, the provider, or the credential is missing.
	ReasonNotConfigured = "not_configured"
	// ReasonNoPublishedRelease means there is nothing released to index yet.
	ReasonNoPublishedRelease = "no_published_release"
)

// ChunkVersion v2 indexes governed wording and five-type names; v1 indexed only name, title and description, so
// knowledge named by displayName was indexed by address alone.
const ChunkVersion = "released-summary/v2"
const MaxChunks = 5000
const BatchSize = 16

type Config struct {
	ProviderID         identity.ModelProviderID `json:"providerId"`
	SettingID          identity.ModelSettingID  `json:"settingId"`
	Endpoint           string                   `json:"endpoint"`
	Model              string                   `json:"model"`
	Dimension          int                      `json:"dimension"`
	CredentialEnv      string                   `json:"credentialEnv"`
	CredentialRevision string                   `json:"credentialRevision"`
	ChunkVersion       string                   `json:"chunkVersion"`
}

func Digest(value any) string {
	data, _ := json.Marshal(value)
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:])
}

type Index struct {
	ID           identity.RunID       `json:"id"`
	WorkspaceID  identity.WorkspaceID `json:"workspaceId"`
	ReleaseID    identity.ReleaseID   `json:"releaseId"`
	Model        string               `json:"model"`
	Dimension    int                  `json:"dimension"`
	CorpusDigest string               `json:"corpusDigest"`
	ChunkCount   int                  `json:"chunkCount"`
	VectorCount  int                  `json:"vectorCount"`
	State        string               `json:"state"`
	ErrorCode    string               `json:"errorCode"`
	RuntimeRunID identity.RunID       `json:"runtimeRunId"`
	JobID        identity.RunID       `json:"-"`
	CreatedAt    time.Time            `json:"createdAt"`
	UpdatedAt    time.Time            `json:"updatedAt"`
	Config       Config               `json:"-"`
}

type Chunk struct {
	Ordinal    int                 `json:"ordinal"`
	AssetID    identity.AssetID    `json:"assetId"`
	RevisionID identity.RevisionID `json:"revisionId"`
	Address    string              `json:"address"`
	Text       string              `json:"text"`
	Digest     string              `json:"digest"`
}

type Candidate struct {
	AssetID    identity.AssetID    `json:"assetId"`
	RevisionID identity.RevisionID `json:"revisionId"`
	Address    string              `json:"address"`
	Score      float64             `json:"score"`
}

type Status struct {
	Configured bool   `json:"configured"`
	Reason     string `json:"reason"`
	Active     *Index `json:"active"`
	Latest     *Index `json:"latest"`
}
type SearchResult struct {
	Mode           string      `json:"mode"`
	FallbackReason string      `json:"fallbackReason"`
	Items          []Candidate `json:"items"`
}

func ValidateVectors(vectors [][]float32, count, dimension int) error {
	if count < 1 || dimension < 1 || dimension > 4096 || len(vectors) != count {
		return ErrInvalid
	}
	for _, vector := range vectors {
		if len(vector) != dimension {
			return ErrInvalid
		}
		norm := float64(0)
		for _, v := range vector {
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				return ErrInvalid
			}
			norm += float64(v) * float64(v)
		}
		if norm == 0 {
			return ErrInvalid
		}
	}
	return nil
}

// ChunkText renders one released revision for embedding: identity and meaning first, then the wording people use.
func ChunkText(address string, content json.RawMessage) (string, error) {
	var fields struct {
		DisplayName     string   `json:"displayName"`
		Name            string   `json:"name"`
		Title           string   `json:"title"`
		Description     string   `json:"description"`
		Definition      string   `json:"definition"`
		Scope           string   `json:"scope"`
		Aliases         []string `json:"aliases"`
		Includes        []string `json:"includes"`
		Excludes        []string `json:"excludes"`
		SampleQuestions []struct {
			Question string `json:"question"`
		} `json:"sampleQuestions"`
		Spec struct {
			Members []struct {
				Name    string   `json:"name"`
				Aliases []string `json:"aliases"`
			} `json:"members"`
		} `json:"spec"`
	}
	if err := json.Unmarshal(content, &fields); err != nil {
		return "", err
	}
	lines := []string{address}
	add := func(label string, values ...string) {
		kept := []string{}
		for _, value := range values {
			if value = strings.TrimSpace(value); value != "" {
				kept = append(kept, value)
			}
		}
		if len(kept) == 0 {
			return
		}
		if label == "" {
			lines = append(lines, kept...)
			return
		}
		lines = append(lines, label+strings.Join(kept, "; "))
	}
	add("", fields.DisplayName, fields.Name, fields.Title, fields.Description, fields.Definition, fields.Scope)
	add("aliases: ", fields.Aliases...)
	add("includes: ", fields.Includes...)
	add("excludes: ", fields.Excludes...)
	questions := []string{}
	for _, sample := range fields.SampleQuestions {
		questions = append(questions, sample.Question)
	}
	add("questions: ", questions...)
	members := []string{}
	for _, member := range fields.Spec.Members {
		members = append(members, strings.Join(append([]string{member.Name}, member.Aliases...), "/"))
	}
	add("members: ", members...)
	text := strings.Join(lines, "\n")
	if len(text) > 2048 {
		text = text[:2048]
		for !utf8.ValidString(text) {
			text = text[:len(text)-1]
		}
	}
	return text, nil
}
