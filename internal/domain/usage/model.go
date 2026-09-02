package usage

import (
	"time"

	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
)

const (
	AssetRead       = "catalog.asset.read"
	SearchCompleted = "catalog.search.completed"
)

type Event struct {
	ID                identity.EventID
	WorkspaceID       identity.WorkspaceID
	EventType         string
	IdempotencyKey    string
	DataVersion       int16
	ActorID           string
	AssetID           *identity.AssetID
	RevisionID        *identity.RevisionID
	Channel           string
	Outcome           string
	ReasonCode        string
	TraceID           string
	SearchFingerprint string
	SearchLanguage    string
	TokenBucket       string
	ResultBucket      string
	AssetTypeFilter   semantic.AssetType
	LifecycleFilter   string
	OccurredAt        time.Time
	ReceivedAt        time.Time
	ExpiresAt         time.Time
}
