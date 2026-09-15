package governance

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

// GovernedObjectRepository is the persistence contract of the four M2
// governance object types. The adapter implements it through one shared
// pipeline; the repository never exposes update or delete outside
// ApplyGovernedChange, which is the only governed mutation path.
type GovernedObjectRepository interface {
	CreateGovernedObject(ctx context.Context, object governance.GovernedObject) (governance.GovernedObject, error)
	GetGovernedObject(ctx context.Context, workspace identity.WorkspaceID, objectType governance.TargetObjectType, objectID string) (governance.GovernedObject, error)
	ListGovernedObjects(ctx context.Context, workspace identity.WorkspaceID, objectType governance.TargetObjectType) ([]governance.GovernedObject, error)
	ApplyGovernedChange(ctx context.Context, command GovernedChangeCommand) (governance.GovernedObject, error)
}

// GovernedChangeCommand carries the pre-minted audit event identity so the
// repository commits the version bump and the audit fact in one transaction.
// There is deliberately no outbox event identity: governed object application
// stays audit-only in this task, release publication remains the outbox
// signal (packet M2-T008).
type GovernedChangeCommand struct {
	WorkspaceID     identity.WorkspaceID
	ObjectType      governance.TargetObjectType
	ObjectID        string
	ExpectedVersion int
	Patch           governance.GovernedPatch
	Actor           string
	TraceID         string
	AuditEventID    identity.EventID
	UpdatedAt       time.Time
}

type GovernedObjectService struct {
	repository GovernedObjectRepository
	clock      Clock
}

func NewGovernedObjectService(repository GovernedObjectRepository, clock Clock) *GovernedObjectService {
	return &GovernedObjectService{repository: repository, clock: clock}
}

type CreateGovernedObjectRequest struct {
	WorkspaceID identity.WorkspaceID
	Object      governance.GovernedObject
	CreatedBy   string
}

// Create persists a new governed object with version 1. Creation is an
// authoring act and emits no event; only ApplyGovernedChange records facts.
func (service *GovernedObjectService) Create(ctx context.Context, request CreateGovernedObjectRequest) (governance.GovernedObject, error) {
	request.CreatedBy = strings.TrimSpace(request.CreatedBy)
	if request.WorkspaceID.IsZero() || request.CreatedBy == "" || len(request.CreatedBy) > 256 {
		return governance.GovernedObject{}, governance.ErrInvalidArgument
	}
	object := request.Object
	if !object.Type.IsGovernedObject() {
		return governance.GovernedObject{}, governance.ErrInvalidArgument
	}
	if err := object.Validate(); err != nil {
		return governance.GovernedObject{}, err
	}
	now := service.clock.Now().UTC()
	switch object.Type {
	case governance.TargetPhysicalBinding:
		id, err := identity.NewPhysicalBindingID()
		if err != nil {
			return governance.GovernedObject{}, fmt.Errorf("mint physical binding ID: %w", err)
		}
		binding := *object.PhysicalBinding
		binding.ID, binding.WorkspaceID, binding.Version = id, request.WorkspaceID, 1
		binding.CreatedBy, binding.CreatedAt, binding.UpdatedAt = request.CreatedBy, now, now
		object.PhysicalBinding = &binding
	case governance.TargetModelGrain:
		id, err := identity.NewModelGrainID()
		if err != nil {
			return governance.GovernedObject{}, fmt.Errorf("mint model grain ID: %w", err)
		}
		grain := *object.ModelGrain
		grain.ID, grain.WorkspaceID, grain.Version = id, request.WorkspaceID, 1
		grain.CreatedBy, grain.CreatedAt, grain.UpdatedAt = request.CreatedBy, now, now
		object.ModelGrain = &grain
	case governance.TargetEntityKey:
		id, err := identity.NewEntityKeyID()
		if err != nil {
			return governance.GovernedObject{}, fmt.Errorf("mint entity key ID: %w", err)
		}
		key := *object.EntityKey
		key.ID, key.WorkspaceID, key.Version = id, request.WorkspaceID, 1
		key.CreatedBy, key.CreatedAt, key.UpdatedAt = request.CreatedBy, now, now
		object.EntityKey = &key
	case governance.TargetJoinContract:
		id, err := identity.NewJoinContractID()
		if err != nil {
			return governance.GovernedObject{}, fmt.Errorf("mint join contract ID: %w", err)
		}
		contract := *object.JoinContract
		contract.ID, contract.WorkspaceID, contract.Version = id, request.WorkspaceID, 1
		contract.CreatedBy, contract.CreatedAt, contract.UpdatedAt = request.CreatedBy, now, now
		object.JoinContract = &contract
	default:
		return governance.GovernedObject{}, governance.ErrInvalidArgument
	}
	return service.repository.CreateGovernedObject(ctx, object)
}

// Get returns one governed object, workspace-scoped by the repository.
func (service *GovernedObjectService) Get(
	ctx context.Context, workspace identity.WorkspaceID, objectType governance.TargetObjectType, objectID string,
) (governance.GovernedObject, error) {
	if workspace.IsZero() || !objectType.IsGovernedObject() || objectID == "" {
		return governance.GovernedObject{}, governance.ErrInvalidArgument
	}
	if _, err := ParseGovernedObjectUUID(objectType, objectID); err != nil {
		return governance.GovernedObject{}, err
	}
	return service.repository.GetGovernedObject(ctx, workspace, objectType, objectID)
}

// List returns the governed objects of one type in one workspace.
func (service *GovernedObjectService) List(
	ctx context.Context, workspace identity.WorkspaceID, objectType governance.TargetObjectType,
) ([]governance.GovernedObject, error) {
	if workspace.IsZero() || !objectType.IsGovernedObject() {
		return nil, governance.ErrInvalidArgument
	}
	return service.repository.ListGovernedObjects(ctx, workspace, objectType)
}

type ApplyGovernedChangeRequest struct {
	WorkspaceID     identity.WorkspaceID
	ObjectType      governance.TargetObjectType
	ObjectID        string
	ExpectedVersion int
	Content         json.RawMessage
	Summary         string
	Actor           string
	TraceID         string
}

// ApplyGovernedChange is the only governed mutation path: the repository
// locks the row FOR UPDATE, verifies the expected version, bumps version by
// exactly one and records the audit fact in the same transaction. Concurrent
// applications are rejected with the invariant error.
func (service *GovernedObjectService) ApplyGovernedChange(ctx context.Context, request ApplyGovernedChangeRequest) (governance.GovernedObject, error) {
	request.Actor = strings.TrimSpace(request.Actor)
	patch := governance.GovernedPatch{Content: request.Content, Summary: request.Summary}
	if request.WorkspaceID.IsZero() || !request.ObjectType.IsGovernedObject() ||
		request.ObjectID == "" || request.ExpectedVersion <= 0 || request.Actor == "" {
		return governance.GovernedObject{}, governance.ErrInvalidArgument
	}
	if _, err := ParseGovernedObjectUUID(request.ObjectType, request.ObjectID); err != nil {
		return governance.GovernedObject{}, err
	}
	if err := patch.Validate(); err != nil {
		return governance.GovernedObject{}, err
	}
	auditEventID, err := identity.NewEventID()
	if err != nil {
		return governance.GovernedObject{}, fmt.Errorf("mint governed change audit ID: %w", err)
	}
	now := service.clock.Now().UTC()
	return service.repository.ApplyGovernedChange(ctx, GovernedChangeCommand{
		WorkspaceID: request.WorkspaceID, ObjectType: request.ObjectType, ObjectID: request.ObjectID,
		ExpectedVersion: request.ExpectedVersion, Patch: patch, Actor: request.Actor,
		TraceID: request.TraceID, AuditEventID: auditEventID, UpdatedAt: now,
	})
}

// ParseGovernedObjectUUID enforces the TypeID prefix of the target type and
// returns the underlying UUID string used for proposal targeting and for the
// repository commands.
func ParseGovernedObjectUUID(objectType governance.TargetObjectType, value string) (string, error) {
	parsed, err := parseGovernedTypedID(objectType, value)
	if err != nil {
		return "", fmt.Errorf("%w: governed object ID %q: %v", governance.ErrInvalidArgument, value, err)
	}
	return parsed.UUID(), nil
}

func parseGovernedTypedID(objectType governance.TargetObjectType, value string) (interface{ UUID() string }, error) {
	switch objectType {
	case governance.TargetPhysicalBinding:
		return identity.ParsePhysicalBindingID(value)
	case governance.TargetModelGrain:
		return identity.ParseModelGrainID(value)
	case governance.TargetEntityKey:
		return identity.ParseEntityKeyID(value)
	case governance.TargetJoinContract:
		return identity.ParseJoinContractID(value)
	default:
		return nil, fmt.Errorf("%q is not a governed object type", objectType)
	}
}
