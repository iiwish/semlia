package usage

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/iiwish/semlia/internal/domain/semantic"
	domain "github.com/iiwish/semlia/internal/domain/usage"
	"github.com/iiwish/semlia/pkg/identity"
)

const retention = 90 * 24 * time.Hour

var ErrInvalid = errors.New("invalid usage event")

type Repository interface {
	RecordUsageEvent(context.Context, domain.Event) error
	DeleteExpiredUsageEvents(context.Context, time.Time, int) (int64, error)
}

type Clock interface{ Now() time.Time }

type ClockFunc func() time.Time

func (clock ClockFunc) Now() time.Time { return clock() }

type Service struct {
	repository Repository
	clock      Clock
	rootSecret []byte
}

func NewService(repository Repository, clock Clock, rootSecret []byte) (*Service, error) {
	if repository == nil || clock == nil || len(rootSecret) < 32 {
		return nil, ErrInvalid
	}
	return &Service{repository: repository, clock: clock, rootSecret: append([]byte(nil), rootSecret...)}, nil
}

type ReadInput struct {
	WorkspaceID identity.WorkspaceID
	AssetID     identity.AssetID
	RevisionID  identity.RevisionID
	ActorID     string
	Channel     string
	TraceID     string
}

func (service *Service) RecordAssetRead(ctx context.Context, input ReadInput) error {
	if input.WorkspaceID.IsZero() || input.AssetID.IsZero() || input.RevisionID.IsZero() ||
		!validContext(input.Channel, input.TraceID) {
		return ErrInvalid
	}
	now := service.clock.Now().UTC()
	id, err := identity.NewEventID()
	if err != nil {
		return err
	}
	return service.repository.RecordUsageEvent(ctx, domain.Event{
		ID: id, WorkspaceID: input.WorkspaceID, EventType: domain.AssetRead,
		IdempotencyKey: input.TraceID + ":" + input.AssetID.String() + ":" + input.RevisionID.String(),
		DataVersion:    1, ActorID: strings.TrimSpace(input.ActorID), AssetID: &input.AssetID,
		RevisionID: &input.RevisionID, Channel: input.Channel, Outcome: "succeeded", TraceID: input.TraceID,
		OccurredAt: now, ReceivedAt: now, ExpiresAt: now.Add(retention),
	})
}

type SearchInput struct {
	WorkspaceID     identity.WorkspaceID
	Query           string
	Language        string
	AssetTypeFilter string
	LifecycleFilter string
	ResultCount     int
	Failed          bool
	ReasonCode      string
	ActorID         string
	Channel         string
	TraceID         string
}

func (service *Service) RecordSearch(ctx context.Context, input SearchInput) error {
	query := strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(input.Query))), " ")
	if input.WorkspaceID.IsZero() || query == "" || len(query) > 256 || input.ResultCount < 0 ||
		!validContext(input.Channel, input.TraceID) || !validLanguage(input.Language) {
		return ErrInvalid
	}
	outcome := "matched"
	reasonCode := ""
	if input.Failed {
		outcome, reasonCode = "failed", input.ReasonCode
		if !validReason(reasonCode) {
			return ErrInvalid
		}
	} else if input.ResultCount == 0 {
		outcome = "zero_result"
	}
	now := service.clock.Now().UTC()
	id, err := identity.NewEventID()
	if err != nil {
		return err
	}
	return service.repository.RecordUsageEvent(ctx, domain.Event{
		ID: id, WorkspaceID: input.WorkspaceID, EventType: domain.SearchCompleted,
		IdempotencyKey: input.TraceID + ":search", DataVersion: 1, ActorID: strings.TrimSpace(input.ActorID),
		Channel: input.Channel, Outcome: outcome, ReasonCode: reasonCode, TraceID: input.TraceID,
		SearchFingerprint: service.fingerprint(input.WorkspaceID, query), SearchLanguage: input.Language,
		TokenBucket: tokenBucket(query), ResultBucket: resultBucket(input.ResultCount),
		AssetTypeFilter: semanticAssetType(input.AssetTypeFilter), LifecycleFilter: input.LifecycleFilter,
		OccurredAt: now, ReceivedAt: now, ExpiresAt: now.Add(retention),
	})
}

func (service *Service) DeleteExpired(ctx context.Context, limit int) (int64, error) {
	if limit < 1 || limit > 10_000 {
		return 0, ErrInvalid
	}
	return service.repository.DeleteExpiredUsageEvents(ctx, service.clock.Now().UTC(), limit)
}

func (service *Service) fingerprint(workspace identity.WorkspaceID, query string) string {
	workspaceMAC := hmac.New(sha256.New, service.rootSecret)
	_, _ = workspaceMAC.Write([]byte(workspace.String()))
	queryMAC := hmac.New(sha256.New, workspaceMAC.Sum(nil))
	_, _ = queryMAC.Write([]byte(query))
	return hex.EncodeToString(queryMAC.Sum(nil))
}

func tokenBucket(query string) string {
	count := 0
	inToken := false
	for _, character := range query {
		if unicode.IsSpace(character) {
			inToken = false
		} else if !inToken {
			count++
			inToken = true
		}
	}
	switch {
	case count <= 1:
		return "1"
	case count <= 3:
		return "2_3"
	case count <= 10:
		return "4_10"
	default:
		return "11_plus"
	}
}

func resultBucket(count int) string {
	switch {
	case count == 0:
		return "0"
	case count <= 10:
		return "1_10"
	case count <= 100:
		return "11_100"
	default:
		return "101_plus"
	}
}

func validContext(channel, traceID string) bool {
	if channel != "api" && channel != "agent" && channel != "system" {
		return false
	}
	if len(traceID) != 32 {
		return false
	}
	_, err := hex.DecodeString(traceID)
	return err == nil && traceID == strings.ToLower(traceID)
}

func validLanguage(value string) bool {
	if value == "" {
		return true
	}
	parts := strings.Split(value, "-")
	if len(parts) > 2 || len(parts[0]) < 2 || len(parts[0]) > 3 {
		return false
	}
	for _, character := range parts[0] {
		if character < 'a' || character > 'z' {
			return false
		}
	}
	if len(parts) == 2 {
		return len(parts[1]) == 2 && parts[1][0] >= 'A' && parts[1][0] <= 'Z' && parts[1][1] >= 'A' && parts[1][1] <= 'Z'
	}
	return true
}

func validReason(value string) bool {
	if len(value) < 3 || len(value) > 64 || value[0] < 'A' || value[0] > 'Z' {
		return false
	}
	for _, character := range value[1:] {
		if (character < 'A' || character > 'Z') && (character < '0' || character > '9') && character != '_' {
			return false
		}
	}
	return true
}

func semanticAssetType(value string) semantic.AssetType { return semantic.AssetType(value) }
