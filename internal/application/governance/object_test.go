package governance_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	governanceapp "github.com/iiwish/semlia/internal/application/governance"
	"github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

type governedObjectRepositoryStub struct {
	created  []governance.GovernedObject
	applied  []governanceapp.GovernedChangeCommand
	nextVer  map[string]int
	failWith error
}

func newGovernedObjectRepositoryStub() *governedObjectRepositoryStub {
	return &governedObjectRepositoryStub{nextVer: map[string]int{}}
}

func (stub *governedObjectRepositoryStub) CreateGovernedObject(
	_ context.Context, object governance.GovernedObject,
) (governance.GovernedObject, error) {
	if stub.failWith != nil {
		return governance.GovernedObject{}, stub.failWith
	}
	stub.created = append(stub.created, object)
	return object, nil
}

func (stub *governedObjectRepositoryStub) GetGovernedObject(
	_ context.Context, _ identity.WorkspaceID, objectType governance.TargetObjectType, objectID string,
) (governance.GovernedObject, error) {
	if stub.failWith != nil {
		return governance.GovernedObject{}, stub.failWith
	}
	if objectType != governance.TargetModelGrain || objectID == "" {
		return governance.GovernedObject{}, governance.ErrNotFound
	}
	grain := governance.ModelGrain{GrainExpression: "one row per order", Content: json.RawMessage(`{}`)}
	return governance.GovernedObject{Type: objectType, ModelGrain: &grain}, nil
}

func (stub *governedObjectRepositoryStub) ListGovernedObjects(
	_ context.Context, _ identity.WorkspaceID, objectType governance.TargetObjectType,
) ([]governance.GovernedObject, error) {
	if stub.failWith != nil {
		return nil, stub.failWith
	}
	return []governance.GovernedObject{{Type: objectType}}, nil
}

func (stub *governedObjectRepositoryStub) ApplyGovernedChange(
	_ context.Context, command governanceapp.GovernedChangeCommand,
) (governance.GovernedObject, error) {
	if stub.failWith != nil {
		return governance.GovernedObject{}, stub.failWith
	}
	stub.applied = append(stub.applied, command)
	grain := governance.ModelGrain{GrainExpression: "one row per order line", Content: command.Patch.Content}
	return governance.GovernedObject{Type: command.ObjectType, ModelGrain: &grain}, nil
}

func TestGovernedObjectServiceCreateMintsTypedIdentityAndVersionOne(t *testing.T) {
	stub := newGovernedObjectRepositoryStub()
	service := governanceapp.NewGovernedObjectService(stub, governanceapp.ClockFunc(
		func() time.Time { return time.Date(2026, 9, 3, 8, 0, 0, 0, time.UTC) }))
	workspaceID, err := identity.NewWorkspaceID()
	if err != nil {
		t.Fatal(err)
	}
	assetID, err := identity.NewAssetID()
	if err != nil {
		t.Fatal(err)
	}
	created, err := service.Create(ctxBackground(), governanceapp.CreateGovernedObjectRequest{
		WorkspaceID: workspaceID,
		Object: governance.GovernedObject{
			Type: governance.TargetModelGrain,
			ModelGrain: &governance.ModelGrain{
				WorkspaceID:     workspaceID,
				AssetID:         assetID,
				GrainExpression: "one row per order",
				GrainFieldRefs:  []identity.PhysicalFieldID{mustRef(t)},
				Content:         json.RawMessage(`{}`),
			},
		},
		CreatedBy: "steward",
	})
	if err != nil {
		t.Fatalf("create governed object: %v", err)
	}
	if created.ModelGrain == nil || created.ModelGrain.ID.IsZero() {
		t.Fatalf("service must mint the model grain identity: %+v", created)
	}
	if created.ModelGrain.ID.Prefix() != identity.ModelGrain {
		t.Fatalf("minted prefix = %s, want mgn", created.ModelGrain.ID.Prefix())
	}
	if created.ModelGrain.Version != 1 || created.ModelGrain.CreatedBy != "steward" || created.ModelGrain.CreatedAt.IsZero() {
		t.Fatalf("create must pin version 1, creator and timestamps: %+v", created.ModelGrain)
	}
}

func TestGovernedObjectServiceRejectsInvalidCommands(t *testing.T) {
	stub := newGovernedObjectRepositoryStub()
	service := governanceapp.NewGovernedObjectService(stub, governanceapp.ClockFunc(
		func() time.Time { return time.Date(2026, 9, 3, 8, 0, 0, 0, time.UTC) }))
	workspaceID, err := identity.NewWorkspaceID()
	if err != nil {
		t.Fatal(err)
	}
	grain := governance.ModelGrain{
		WorkspaceID:     workspaceID,
		AssetID:         func() identity.AssetID { id, _ := identity.NewAssetID(); return id }(),
		GrainExpression: "one row per order",
		GrainFieldRefs:  []identity.PhysicalFieldID{mustRef(t)},
		Content:         json.RawMessage(`{}`),
	}
	cases := map[string]governanceapp.CreateGovernedObjectRequest{
		"missing workspace": {
			Object:    governance.GovernedObject{Type: governance.TargetModelGrain, ModelGrain: &grain},
			CreatedBy: "steward",
		},
		"missing creator": {
			WorkspaceID: workspaceID,
			Object:      governance.GovernedObject{Type: governance.TargetModelGrain, ModelGrain: &grain},
		},
		"unknown object type": {
			WorkspaceID: workspaceID,
			Object:      governance.GovernedObject{Type: "deployed", ModelGrain: &grain},
			CreatedBy:   "steward",
		},
		"asset pipeline type": {
			WorkspaceID: workspaceID,
			Object:      governance.GovernedObject{Type: governance.TargetSemanticAsset, ModelGrain: &grain},
			CreatedBy:   "steward",
		},
	}
	for name, request := range cases {
		if _, err := service.Create(ctxBackground(), request); !errors.Is(err, governance.ErrInvalidArgument) {
			t.Fatalf("%s: create accepted (%v), want ErrInvalidArgument", name, err)
		}
	}
	if len(stub.created) != 0 {
		t.Fatalf("invalid creates reached the repository: %d", len(stub.created))
	}
}

func TestGovernedObjectServiceApplyMintsAuditIdentityAndValidatesCommand(t *testing.T) {
	stub := newGovernedObjectRepositoryStub()
	service := governanceapp.NewGovernedObjectService(stub, governanceapp.ClockFunc(
		func() time.Time { return time.Date(2026, 9, 3, 8, 0, 0, 0, time.UTC) }))
	workspaceID, err := identity.NewWorkspaceID()
	if err != nil {
		t.Fatal(err)
	}
	grainID, err := identity.NewModelGrainID()
	if err != nil {
		t.Fatal(err)
	}
	applied, err := service.ApplyGovernedChange(ctxBackground(), governanceapp.ApplyGovernedChangeRequest{
		WorkspaceID: workspaceID, ObjectType: governance.TargetModelGrain, ObjectID: grainID.String(),
		ExpectedVersion: 1, Content: json.RawMessage(`{"grain":"per order line"}`), Summary: "tighten grain",
		Actor: "steward", TraceID: "4bf92f3577b34da6a3ce929d0e0e4736",
	})
	if err != nil {
		t.Fatalf("apply governed change: %v", err)
	}
	if applied.ModelGrain == nil {
		t.Fatalf("apply must return the updated object: %+v", applied)
	}
	if len(stub.applied) != 1 {
		t.Fatalf("repository applications = %d, want 1", len(stub.applied))
	}
	command := stub.applied[0]
	if command.AuditEventID.IsZero() {
		t.Fatal("apply must pre-mint the audit event identity")
	}
	if command.ExpectedVersion != 1 || command.Patch.Summary != "tighten grain" {
		t.Fatalf("apply command = %+v", command)
	}

	invalid := map[string]governanceapp.ApplyGovernedChangeRequest{
		"missing workspace":     {ObjectType: governance.TargetModelGrain, ObjectID: grainID.String(), ExpectedVersion: 1, Summary: "s", Content: json.RawMessage(`{}`), Actor: "a"},
		"unknown object type":   {WorkspaceID: workspaceID, ObjectType: "deployed", ObjectID: grainID.String(), ExpectedVersion: 1, Summary: "s", Content: json.RawMessage(`{}`), Actor: "a"},
		"asset pipeline type":   {WorkspaceID: workspaceID, ObjectType: governance.TargetSemanticAsset, ObjectID: grainID.String(), ExpectedVersion: 1, Summary: "s", Content: json.RawMessage(`{}`), Actor: "a"},
		"missing object id":     {WorkspaceID: workspaceID, ObjectType: governance.TargetModelGrain, ExpectedVersion: 1, Summary: "s", Content: json.RawMessage(`{}`), Actor: "a"},
		"non-positive version":  {WorkspaceID: workspaceID, ObjectType: governance.TargetModelGrain, ObjectID: grainID.String(), ExpectedVersion: 0, Summary: "s", Content: json.RawMessage(`{}`), Actor: "a"},
		"missing actor":         {WorkspaceID: workspaceID, ObjectType: governance.TargetModelGrain, ObjectID: grainID.String(), ExpectedVersion: 1, Summary: "s", Content: json.RawMessage(`{}`)},
		"missing patch summary": {WorkspaceID: workspaceID, ObjectType: governance.TargetModelGrain, ObjectID: grainID.String(), ExpectedVersion: 1, Content: json.RawMessage(`{}`), Actor: "a"},
		"non-object content":    {WorkspaceID: workspaceID, ObjectType: governance.TargetModelGrain, ObjectID: grainID.String(), ExpectedVersion: 1, Summary: "s", Content: json.RawMessage(`[]`), Actor: "a"},
	}
	for name, request := range invalid {
		if _, err := service.ApplyGovernedChange(ctxBackground(), request); !errors.Is(err, governance.ErrInvalidArgument) {
			t.Fatalf("%s: apply accepted (%v), want ErrInvalidArgument", name, err)
		}
	}
	if len(stub.applied) != 1 {
		t.Fatalf("invalid applies reached the repository: %d", len(stub.applied))
	}
}

func mustRef(t *testing.T) identity.PhysicalFieldID {
	t.Helper()
	ref, err := identity.NewPhysicalFieldID()
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func ctxBackground() context.Context { return context.Background() }
