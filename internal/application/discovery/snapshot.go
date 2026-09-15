package discovery

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/iiwish/semlia/internal/domain/authorization"
	domain "github.com/iiwish/semlia/internal/domain/discovery"
	"github.com/iiwish/semlia/pkg/identity"
)

var ErrSnapshotCursor = errors.New("invalid source snapshot cursor")

type SnapshotRequest struct {
	SourceRequest
	SnapshotID string
	Kind       string
	Cursor     string
	Limit      int
}

type SnapshotCursor struct {
	Version   int       `json:"v"`
	Kind      string    `json:"kind"`
	Workspace string    `json:"workspace"`
	Source    string    `json:"source"`
	Snapshot  string    `json:"snapshot"`
	Filter    string    `json:"filter"`
	UpperTime time.Time `json:"upperTime"`
	UpperKey  string    `json:"upperKey"`
	LastTime  time.Time `json:"lastTime"`
	LastKey   string    `json:"lastKey"`
}

type SnapshotReadQuery struct {
	WorkspaceID          identity.WorkspaceID
	SourceID             identity.SourceConnectionID
	SnapshotID           string
	AuthorizationVersion int64
	Kind                 string
	Filter               string
	Limit                int
	Cursor               SnapshotCursor
}

type SnapshotReadResult struct {
	Snapshots      []domain.SourceSnapshot
	Members        []domain.SnapshotMember
	Diagnostics    []domain.SnapshotDiagnostic
	HistoryQuality string
	Cursor         SnapshotCursor
}

type SnapshotRepository interface {
	LookupSourceSnapshot(context.Context, identity.WorkspaceID, identity.SourceConnectionID, string) error
	ReadSourceSnapshot(context.Context, SnapshotReadQuery) (SnapshotReadResult, error)
}

func newSnapshotCursorKey() []byte {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		panic("snapshot cursor entropy unavailable")
	}
	return key
}

func (service *ControlService) WithSnapshotCursorSecret(secret []byte) *ControlService {
	if len(secret) >= 32 {
		mac := hmac.New(sha256.New, secret)
		mac.Write([]byte("semlia.source-snapshot-cursor/v1"))
		service.snapshotCursorKey = mac.Sum(nil)
	}
	return service
}

func (service *ControlService) readSnapshot(ctx context.Context, request SnapshotRequest, kind string) (SnapshotReadResult, error) {
	repository, ok := service.repository.(SnapshotRepository)
	if !ok {
		return SnapshotReadResult{}, domain.ErrNotFound
	}
	if request.SnapshotID != "" {
		if _, err := identity.Parse(identity.SourceSnapshot, request.SnapshotID); err != nil {
			return SnapshotReadResult{}, domain.ErrInvalidInput
		}
	}
	if request.Limit == 0 {
		request.Limit = 50
	}
	if request.Limit < 1 || request.Limit > 200 {
		return SnapshotReadResult{}, domain.ErrInvalidInput
	}
	if request.Kind != "" && request.Kind != "dataset" && request.Kind != "field" && request.Kind != "code" && request.Kind != "lineage" {
		return SnapshotReadResult{}, domain.ErrInvalidInput
	}
	if kind != "members" && request.Kind != "" {
		return SnapshotReadResult{}, domain.ErrInvalidInput
	}
	if kind != "list" && request.SnapshotID == "" {
		return SnapshotReadResult{}, domain.ErrInvalidInput
	}
	// Relation lookup returns no snapshot contents before authorization.
	if err := repository.LookupSourceSnapshot(ctx, request.WorkspaceID, request.SourceID, request.SnapshotID); err != nil {
		return SnapshotReadResult{}, err
	}
	if service.authorizer == nil {
		return SnapshotReadResult{}, domain.ErrForbidden
	}
	version, err := service.authorizeVersion(ctx, request.WorkspaceID, authorization.ActionSourceRead, authorization.Resource{Type: authorization.ScopeSource, ID: request.SourceID.UUID()}, &request.PrincipalRef, request.TraceID)
	if err != nil {
		return SnapshotReadResult{}, err
	}
	cursor := SnapshotCursor{Version: 1, Kind: kind, Workspace: request.WorkspaceID.String(), Source: request.SourceID.String(), Snapshot: request.SnapshotID, Filter: request.Kind}
	if request.Cursor != "" {
		cursor, err = service.decodeSnapshotCursor(request.Cursor, cursor)
		if err != nil {
			return SnapshotReadResult{}, err
		}
	}
	return repository.ReadSourceSnapshot(ctx, SnapshotReadQuery{WorkspaceID: request.WorkspaceID, SourceID: request.SourceID, SnapshotID: request.SnapshotID, AuthorizationVersion: version, Kind: kind, Filter: request.Kind, Limit: request.Limit, Cursor: cursor})
}

func (service *ControlService) ListSnapshots(ctx context.Context, request SnapshotRequest) (domain.SnapshotPage, error) {
	result, err := service.readSnapshot(ctx, request, "list")
	if err != nil {
		return domain.SnapshotPage{}, err
	}
	limit := request.Limit
	if limit == 0 {
		limit = 50
	}
	items, more, err := boundedSnapshotItems(result.Snapshots, limit)
	if err != nil {
		return domain.SnapshotPage{}, err
	}
	page := domain.SnapshotPage{Items: items}
	if more {
		last := items[len(items)-1]
		id, _ := identity.ParseAny(last.ID)
		result.Cursor.LastTime = last.CreatedAt
		result.Cursor.LastKey = id.UUID()
		page.NextCursor = service.encodeSnapshotCursor(result.Cursor)
	}
	return page, nil
}
func (service *ControlService) GetSnapshot(ctx context.Context, request SnapshotRequest) (domain.SourceSnapshot, error) {
	result, err := service.readSnapshot(ctx, request, "detail")
	if err != nil {
		return domain.SourceSnapshot{}, err
	}
	if len(result.Snapshots) != 1 {
		return domain.SourceSnapshot{}, domain.ErrNotFound
	}
	return result.Snapshots[0], nil
}
func (service *ControlService) ListSnapshotMembers(ctx context.Context, request SnapshotRequest) (domain.SnapshotMemberPage, error) {
	result, err := service.readSnapshot(ctx, request, "members")
	if err != nil {
		return domain.SnapshotMemberPage{}, err
	}
	limit := request.Limit
	if limit == 0 {
		limit = 50
	}
	items, more, err := boundedSnapshotItems(result.Members, limit)
	if err != nil {
		return domain.SnapshotMemberPage{}, err
	}
	page := domain.SnapshotMemberPage{SnapshotID: request.SnapshotID, HistoryQuality: result.HistoryQuality, Items: items}
	if more {
		last := items[len(items)-1]
		id, _ := identity.ParseAny(last.ObjectID)
		result.Cursor.LastKey = last.Kind + ":" + id.UUID()
		page.NextCursor = service.encodeSnapshotCursor(result.Cursor)
	}
	return page, nil
}
func (service *ControlService) ListSnapshotDiagnostics(ctx context.Context, request SnapshotRequest) (domain.SnapshotDiagnosticPage, error) {
	result, err := service.readSnapshot(ctx, request, "diagnostics")
	if err != nil {
		return domain.SnapshotDiagnosticPage{}, err
	}
	limit := request.Limit
	if limit == 0 {
		limit = 50
	}
	items, more, err := boundedSnapshotItems(result.Diagnostics, limit)
	if err != nil {
		return domain.SnapshotDiagnosticPage{}, err
	}
	page := domain.SnapshotDiagnosticPage{SnapshotID: request.SnapshotID, Items: items}
	if more {
		encoded, _ := json.Marshal(items[len(items)-1].Ordinal)
		result.Cursor.LastKey = string(encoded)
		page.NextCursor = service.encodeSnapshotCursor(result.Cursor)
	}
	return page, nil
}

func boundedSnapshotItems[T any](items []T, limit int) ([]T, bool, error) {
	result := make([]T, 0, min(len(items), limit))
	size := 4096
	for _, item := range items {
		encoded, err := json.Marshal(item)
		if err != nil {
			return nil, false, domain.ErrInvalidSnapshot
		}
		if len(result) == limit || size+len(encoded)+1 > domain.SnapshotPageBytes {
			if len(result) == 0 {
				return nil, false, domain.ErrInvalidSnapshot
			}
			return result, true, nil
		}
		size += len(encoded) + 1
		result = append(result, item)
	}
	return result, false, nil
}
func (service *ControlService) encodeSnapshotCursor(cursor SnapshotCursor) *string {
	payload, _ := json.Marshal(cursor)
	mac := hmac.New(sha256.New, service.snapshotCursorKey)
	mac.Write(payload)
	token := base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return &token
}
func (service *ControlService) decodeSnapshotCursor(token string, expected SnapshotCursor) (SnapshotCursor, error) {
	if len(token) > 2048 {
		return SnapshotCursor{}, ErrSnapshotCursor
	}
	payload, signature, ok := strings.Cut(token, ".")
	if !ok {
		return SnapshotCursor{}, ErrSnapshotCursor
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return SnapshotCursor{}, ErrSnapshotCursor
	}
	sig, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil {
		return SnapshotCursor{}, ErrSnapshotCursor
	}
	mac := hmac.New(sha256.New, service.snapshotCursorKey)
	mac.Write(raw)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return SnapshotCursor{}, ErrSnapshotCursor
	}
	var cursor SnapshotCursor
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&cursor) != nil || decoder.Decode(&struct{}{}) != io.EOF || cursor.Version != 1 || cursor.Kind != expected.Kind || cursor.Workspace != expected.Workspace || cursor.Source != expected.Source || cursor.Snapshot != expected.Snapshot || cursor.Filter != expected.Filter || cursor.UpperKey == "" || cursor.LastKey == "" {
		return SnapshotCursor{}, ErrSnapshotCursor
	}
	return cursor, nil
}
