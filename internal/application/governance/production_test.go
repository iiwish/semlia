package governance_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	app "github.com/iiwish/semlia/internal/application/governance"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

type mockProductionRepo struct {
	operations   map[string]domain.ProductionOperation
	versions     map[string]domain.ProductionVersion
	targets      map[string][]domain.ProductionTarget
	links        map[string][]domain.ProductionCandidateLink
	contributors map[string][]domain.ProductionContributor
	reservations map[string]domain.ProductionIdentityReservation
	commands     map[string]domain.ProductionCommand
	claims       map[string]domain.ProductionRequestClaim
}

func (m *mockProductionRepo) CheckProductionInput(context.Context, domain.ProductionVersion, []domain.ProductionTarget) error {
	return nil
}

func (m *mockProductionRepo) PrepareProductionBaseline(context.Context, identity.WorkspaceID, identity.PrincipalID, []domain.TargetDeclaration) (domain.ProductionBaseline, error) {
	return domain.ProductionBaseline{Head: domain.HeadReference{Presence: "absent"}, CanonicalJSON: json.RawMessage(`{"head":{"presence":"absent"},"pins":[]}`), Targets: map[string]domain.ProductionTargetBaseline{}}, nil
}

func newMockProductionRepo() *mockProductionRepo {
	return &mockProductionRepo{
		operations:   make(map[string]domain.ProductionOperation),
		versions:     make(map[string]domain.ProductionVersion),
		targets:      make(map[string][]domain.ProductionTarget),
		links:        make(map[string][]domain.ProductionCandidateLink),
		contributors: make(map[string][]domain.ProductionContributor),
		reservations: make(map[string]domain.ProductionIdentityReservation),
		commands:     make(map[string]domain.ProductionCommand),
		claims:       make(map[string]domain.ProductionRequestClaim),
	}
}

func (m *mockProductionRepo) CreateProductionOperationTx(
	ctx context.Context,
	op domain.ProductionOperation,
	version domain.ProductionVersion,
	targets []domain.ProductionTarget,
	links []domain.ProductionCandidateLink,
	contributors []domain.ProductionContributor,
	reservations []domain.ProductionIdentityReservation,
	cmd domain.ProductionCommand,
	claim domain.ProductionRequestClaim,
	proposals []domain.Proposal,
	assetDrafts []app.SemanticAssetDraft,
) error {
	m.operations[op.ID.String()] = op
	m.versions[op.ID.String()] = version
	m.targets[op.ID.String()] = targets
	m.links[op.ID.String()] = links
	m.contributors[op.ID.String()] = contributors
	for _, r := range reservations {
		m.reservations[r.Kind+":"+r.IdentityKey] = r
	}
	m.commands[cmd.CommandKind+":"+cmd.IdempotencyKey] = cmd
	m.claims[claim.BusinessDigest] = claim
	return nil
}

func (m *mockProductionRepo) GetProductionOperation(
	ctx context.Context,
	workspace identity.WorkspaceID,
	opID identity.ProductionOperationID,
) (domain.ProductionOperation, domain.ProductionVersion, []domain.ProductionTarget, []domain.ProductionCandidateLink, []domain.ProductionContributor, error) {
	op, ok := m.operations[opID.String()]
	if !ok {
		return domain.ProductionOperation{}, domain.ProductionVersion{}, nil, nil, nil, domain.ErrNotFound
	}
	return op, m.versions[opID.String()], m.targets[opID.String()], m.links[opID.String()], m.contributors[opID.String()], nil
}

func (m *mockProductionRepo) GetProductionOperationVersion(ctx context.Context, workspace identity.WorkspaceID, opID identity.ProductionOperationID, version int) (domain.ProductionOperation, domain.ProductionVersion, []domain.ProductionTarget, []domain.ProductionCandidateLink, []domain.ProductionContributor, error) {
	op, ver, targets, links, contributors, err := m.GetProductionOperation(ctx, workspace, opID)
	if err != nil || ver.Version != version {
		return domain.ProductionOperation{}, domain.ProductionVersion{}, nil, nil, nil, domain.ErrNotFound
	}
	return op, ver, targets, links, contributors, nil
}

func (m *mockProductionRepo) ListProductionOperationsPage(ctx context.Context, workspace identity.WorkspaceID, query domain.ProductionListQuery) ([]domain.ProductionOperation, error) {
	return m.ListProductionOperations(ctx, workspace, query.Limit, 0)
}

func (m *mockProductionRepo) ListProductionOperations(
	ctx context.Context,
	workspace identity.WorkspaceID,
	limit int,
	offset int,
) ([]domain.ProductionOperation, error) {
	var list []domain.ProductionOperation
	for _, op := range m.operations {
		list = append(list, op)
	}
	return list, nil
}

func (m *mockProductionRepo) GetProductionCommand(
	ctx context.Context,
	workspace identity.WorkspaceID,
	principal identity.PrincipalID,
	cmdKind string,
	idempotencyKey string,
) (*domain.ProductionCommand, error) {
	cmd, ok := m.commands[cmdKind+":"+idempotencyKey]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return &cmd, nil
}

func (m *mockProductionRepo) GetProductionRequestClaim(
	ctx context.Context,
	workspace identity.WorkspaceID,
	businessDigest string,
) (*domain.ProductionRequestClaim, error) {
	claim, ok := m.claims[businessDigest]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return &claim, nil
}

func (m *mockProductionRepo) GetProductionReservation(
	ctx context.Context,
	workspace identity.WorkspaceID,
	kind string,
	identityKey string,
) (*domain.ProductionIdentityReservation, error) {
	res, ok := m.reservations[kind+":"+identityKey]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return &res, nil
}

func (m *mockProductionRepo) ReplaceDraftTx(
	ctx context.Context,
	opID identity.ProductionOperationID,
	newVersion domain.ProductionVersion,
	targets []domain.ProductionTarget,
	links []domain.ProductionCandidateLink,
	cmd domain.ProductionCommand,
	proposals []domain.Proposal,
) error {
	m.versions[opID.String()] = newVersion
	m.targets[opID.String()] = targets
	m.links[opID.String()] = links
	return nil
}

func (m *mockProductionRepo) SubmitOperationTx(
	ctx context.Context,
	workspace identity.WorkspaceID,
	opID identity.ProductionOperationID,
	version int,
	submittedAt time.Time,
	attempt domain.ValidationAttempt,
	runs []domain.ValidationRun,
	bindings []domain.ValidationBinding,
	proposals []domain.Proposal,
	cmd domain.ProductionCommand,
) error {
	return nil
}

func (m *mockProductionRepo) CreateValidationAttemptTx(
	ctx context.Context,
	workspace identity.WorkspaceID,
	opID identity.ProductionOperationID,
	version int,
	attempt domain.ValidationAttempt,
	runs []domain.ValidationRun,
	bindings []domain.ValidationBinding,
	cmd domain.ProductionCommand,
) error {
	return nil
}

func (m *mockProductionRepo) GetValidationAttempt(
	ctx context.Context,
	workspace identity.WorkspaceID,
	opID identity.ProductionOperationID,
	version int,
	attemptNo int,
) (*domain.ValidationAttempt, error) {
	return nil, domain.ErrNotFound
}

func (m *mockProductionRepo) GetLatestValidationAttempt(
	ctx context.Context,
	workspace identity.WorkspaceID,
	opID identity.ProductionOperationID,
	version int,
) (*domain.ValidationAttempt, error) {
	return nil, domain.ErrNotFound
}

func (m *mockProductionRepo) ListValidationAttempts(
	ctx context.Context,
	workspace identity.WorkspaceID,
	opID identity.ProductionOperationID,
	version int,
	limit int,
	offset int,
) ([]domain.ValidationAttempt, error) {
	return nil, nil
}

func (m *mockProductionRepo) ReviewOperationTx(
	ctx context.Context,
	workspace identity.WorkspaceID,
	opID identity.ProductionOperationID,
	version int,
	attemptNo int,
	setDigest string,
	validationDigest string,
	reviews []domain.Review,
	bindings []domain.ProductionReviewBinding,
	proposals []domain.Proposal,
	cmd domain.ProductionCommand,
) error {
	return nil
}

func (m *mockProductionRepo) PublishOperationTx(
	ctx context.Context,
	workspace identity.WorkspaceID,
	release domain.Release,
	releaseProposals []domain.ReleaseProposal,
	manifest domain.ProductionReleaseManifest,
	beforePins []domain.ProductionReleaseBeforePin,
	bindingInputs []domain.ProductionReleaseBindingInput,
	proposals []domain.Proposal,
	cmd domain.ProductionCommand,
) error {
	return nil
}

func (m *mockProductionRepo) RollbackOperationTx(
	ctx context.Context,
	workspace identity.WorkspaceID,
	release domain.Release,
	releaseProposals []domain.ReleaseProposal,
	manifest domain.ProductionReleaseManifest,
	beforePins []domain.ProductionReleaseBeforePin,
	cmd domain.ProductionCommand,
) error {
	return nil
}

func (m *mockProductionRepo) GetLatestRelease(
	ctx context.Context,
	workspace identity.WorkspaceID,
) (*domain.Release, error) {
	return nil, nil
}

func (m *mockProductionRepo) GetRelease(
	ctx context.Context,
	workspace identity.WorkspaceID,
	releaseID identity.ReleaseID,
) (domain.Release, error) {
	return domain.Release{}, domain.ErrNotFound
}

func (m *mockProductionRepo) GetProductionRelease(
	ctx context.Context,
	workspace identity.WorkspaceID,
	releaseID identity.ReleaseID,
) (*domain.Release, *domain.ProductionReleaseManifest, []domain.ProductionReleaseBeforePin, []domain.ReleaseProposal, error) {
	return nil, nil, nil, nil, domain.ErrNotFound
}

func TestProductionServiceCreateAndIdempotency(t *testing.T) {
	repo := newMockProductionRepo()
	service := app.NewProductionService(repo)
	ctx := context.Background()

	wsp, _ := identity.NewWorkspaceID()
	prn, _ := identity.NewPrincipalID()
	firstIdentity := "default.orders"
	content := func(address, name string) json.RawMessage {
		return json.RawMessage(fmt.Sprintf(`{"address":%q,"assetType":"entity","displayName":%q,"definition":null,"scope":null,"ownerPrincipalId":%q}`, address, name, prn.String()))
	}

	cmd := app.CreateOperationCommand{
		WorkspaceID:    wsp,
		PrincipalID:    prn,
		IdempotencyKey: "idem-key-01",
		Targets: []domain.TargetDeclaration{
			{
				LocalKey: "orders",
				Kind:     domain.TargetKindSemanticAsset,
				Intent:   domain.ProductionIntentCreate,
				Title:    "Orders", IdentityKey: &firstIdentity,
				Content: content(firstIdentity, "Orders"),
			},
		},
		Candidates: []domain.CandidateDeclaration{
			{
				CandidateID:      "cand_1",
				CandidateDigest:  "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				TargetKeys:       []string{"orders"},
				PrimaryTargetKey: "orders",
			},
		},
	}

	// 1. Initial creation succeeds
	res1, err := service.CreateOperation(ctx, cmd)
	if err != nil {
		t.Fatalf("create operation failed: %v", err)
	}
	if res1.Replayed {
		t.Fatal("first call should not be replayed")
	}
	if len(res1.Targets) != 1 || res1.Targets[0].Outcome != domain.ProductionOutcomeProposal {
		t.Fatalf("unexpected targets result: %+v", res1.Targets)
	}

	// 2. Replay returns same operation with Replayed=true
	res2, err := service.CreateOperation(ctx, cmd)
	if err != nil {
		t.Fatalf("replay operation failed: %v", err)
	}
	if !res2.Replayed {
		t.Fatal("second call should be replayed")
	}
	if res2.OperationID != res1.OperationID {
		t.Fatalf("replayed operation ID mismatch: %s vs %s", res2.OperationID, res1.OperationID)
	}

	// 3. Different idempotency key but same business input returns ErrAlreadyProduced
	cmd2 := cmd
	cmd2.IdempotencyKey = "idem-key-02"
	_, err = service.CreateOperation(ctx, cmd2)
	if !errors.Is(err, domain.ErrAlreadyProduced) {
		t.Fatalf("expected ErrAlreadyProduced, got: %v", err)
	}

	// 4. Identity collision returns ErrIdentityConflict
	idKey := "sales.orders"
	cmd3 := cmd
	cmd3.IdempotencyKey = "idem-key-03"
	cmd3.Targets = []domain.TargetDeclaration{
		{
			LocalKey:    "different_orders",
			Kind:        domain.TargetKindSemanticAsset,
			Intent:      domain.ProductionIntentCreate,
			IdentityKey: &idKey,
			Title:       "Different", Content: content(idKey, "Different"),
		},
	}
	cmd3.Candidates[0].TargetKeys = []string{"different_orders"}
	cmd3.Candidates[0].PrimaryTargetKey = "different_orders"

	_, err = service.CreateOperation(ctx, cmd3)
	if err != nil {
		t.Fatalf("first creation with identityKey failed: %v", err)
	}

	// Attempting to reserve same identityKey with different content and localKey
	cmd4 := cmd3
	cmd4.IdempotencyKey = "idem-key-04"
	cmd4.Targets = []domain.TargetDeclaration{
		{
			LocalKey:    "competing_orders",
			Kind:        domain.TargetKindSemanticAsset,
			Intent:      domain.ProductionIntentCreate,
			IdentityKey: &idKey,
			Title:       "Competing", Content: content(idKey, "Competing"),
		},
	}
	cmd4.Candidates[0].TargetKeys = []string{"competing_orders"}
	cmd4.Candidates[0].PrimaryTargetKey = "competing_orders"

	_, err = service.CreateOperation(ctx, cmd4)
	if !errors.Is(err, domain.ErrIdentityConflict) {
		t.Fatalf("expected ErrIdentityConflict, got: %v", err)
	}
}
