package db_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	pgstore "github.com/iiwish/semlia/internal/adapters/postgres"
	governanceapp "github.com/iiwish/semlia/internal/application/governance"
	"github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5/pgxpool"
)

const governanceTraceID = "4bf92f3577b34da6a3ce929d0e0e4736"

var governanceClock = governanceapp.ClockFunc(func() time.Time {
	return time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
})

type governanceFixture struct {
	WorkspaceID identity.WorkspaceID
	AssetID     identity.AssetID
	RevisionID  identity.RevisionID
	SecondRevID identity.RevisionID
}

func seedGovernanceAsset(t *testing.T, pool *pgstore.Pool, slug string) governanceFixture {
	t.Helper()
	ctx := context.Background()
	workspaceID := newWorkspaceID(t)
	if _, err := pool.Exec(ctx, `
		INSERT INTO workspaces (id, slug, display_name)
		VALUES ($1, $2, $3)`, workspaceID.UUID(), slug, "Governance "+slug); err != nil {
		t.Fatal(err)
	}
	assetID, err := identity.NewAssetID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO semantic_assets (id, workspace_id, namespace, key, asset_type, lifecycle_state)
		VALUES ($1, $2, 'finance', $3, 'metric', 'draft')`,
		assetID.UUID(), workspaceID.UUID(), slug+"_metric"); err != nil {
		t.Fatal(err)
	}
	revision := func(sequence int) identity.RevisionID {
		id, revisionErr := identity.NewRevisionID()
		if revisionErr != nil {
			t.Fatal(revisionErr)
		}
		if _, revisionErr := pool.Exec(ctx, `
			INSERT INTO asset_revisions (id, workspace_id, asset_id, sequence, schema_version, content_digest, content, created_by)
			VALUES ($1, $2, $3, $4, '1.0.0', $5, '{"definition":{"formula":"revenue"}}'::jsonb, 'steward')`,
			id.UUID(), workspaceID.UUID(), assetID.UUID(), sequence,
			fmt.Sprintf("sha256:%064x", sequence)); revisionErr != nil {
			t.Fatal(revisionErr)
		}
		return id
	}
	first := revision(1)
	second := revision(2)
	return governanceFixture{WorkspaceID: workspaceID, AssetID: assetID, RevisionID: first, SecondRevID: second}
}

func governanceServices(pool *pgstore.Pool) (
	*governanceapp.ProposalService,
	*governanceapp.ValidationService,
	*governanceapp.PolicyService,
	*governanceapp.ReleaseService,
	*governanceapp.AgentRunService,
) {
	store := pgstore.NewStore(pool)
	return governanceapp.NewProposalService(store, governanceClock),
		governanceapp.NewValidationService(store, governanceClock),
		governanceapp.NewPolicyService(store, governanceClock),
		governanceapp.NewReleaseService(store, governanceClock),
		governanceapp.NewAgentRunService(store, governanceClock)
}

func sha256FixtureDigest(seed string) string {
	sum := sha256.Sum256([]byte(seed))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func submitTestProposal(
	t *testing.T,
	ctx context.Context,
	proposalService *governanceapp.ProposalService,
	fixture governanceFixture,
) (identity.ProposalID, identity.ProposalChangeID) {
	t.Helper()
	proposal, err := proposalService.CreateProposal(ctx, governanceapp.CreateProposalRequest{
		WorkspaceID: fixture.WorkspaceID, AssetID: fixture.AssetID, BaseRevisionID: fixture.RevisionID,
		Title: "Tighten revenue formula", Summary: "Exclude one-off discounts", Reason: "Finance sign-off",
		CreatedBy: "steward", TraceID: governanceTraceID,
	})
	if err != nil {
		t.Fatalf("create proposal: %v", err)
	}
	item, err := proposalService.AddChange(ctx, governanceapp.AddChangeRequest{
		WorkspaceID: fixture.WorkspaceID, ProposalID: proposal.ID,
		Item: governance.ChangeSetItem{
			FieldPath: "definition.formula", Op: governance.ChangeUpdate,
			BeforeDigest: sha256FixtureDigest("before"), AfterDigest: sha256FixtureDigest("after"),
			BeforeValue: json.RawMessage(`"revenue"`), AfterValue: json.RawMessage(`"revenue_net"`),
		},
	})
	if err != nil {
		t.Fatalf("add change: %v", err)
	}
	submitted, err := proposalService.Submit(ctx, governanceapp.SubmitRequest{
		WorkspaceID: fixture.WorkspaceID, ProposalID: proposal.ID, Actor: "steward", TraceID: governanceTraceID,
	})
	if err != nil {
		t.Fatalf("submit proposal: %v", err)
	}
	if submitted.State != governance.ProposalProposed || submitted.SubmittedAt == nil {
		t.Fatalf("submitted proposal = %+v", submitted)
	}
	return proposal.ID, item.ID
}

func decodeOutboxEnvelope(t *testing.T, payload []byte) (map[string]json.RawMessage, map[string]any) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var envelope struct {
		SpecVersion string          `json:"specVersion"`
		ID          string          `json:"id"`
		Type        string          `json:"type"`
		Source      string          `json:"source"`
		WorkspaceID string          `json:"workspaceId"`
		Time        string          `json:"time"`
		TraceID     string          `json:"traceId"`
		Data        json.RawMessage `json:"data"`
	}
	if err := decoder.Decode(&envelope); err != nil {
		t.Fatalf("decode outbox envelope: %v", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		t.Fatalf("trailing envelope content: %v", err)
	}
	if envelope.SpecVersion != "semlia.events/v1" || envelope.Type == "" ||
		envelope.Source != "urn:semlia:control-plane" || envelope.WorkspaceID == "" || envelope.TraceID != governanceTraceID {
		t.Fatalf("envelope header invalid: %+v", envelope)
	}
	if _, err := time.Parse(time.RFC3339Nano, envelope.Time); err != nil {
		t.Fatalf("envelope time %q: %v", envelope.Time, err)
	}
	dataDecoder := json.NewDecoder(bytes.NewReader(envelope.Data))
	dataDecoder.DisallowUnknownFields()
	var data map[string]any
	if err := dataDecoder.Decode(&data); err != nil {
		t.Fatalf("decode event data: %v", err)
	}
	if err := dataDecoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		t.Fatalf("trailing event data: %v", err)
	}
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(payload, &raw); err != nil {
		t.Fatalf("raw envelope decode: %v", err)
	}
	return raw, data
}

func TestProposalSubmissionCommitsAuditAndOutboxAtomically(t *testing.T) {
	resetSchema(t)
	pool := openPool(t)
	ctx := context.Background()
	fixture := seedGovernanceAsset(t, pool, "atomic")
	proposalService, _, _, _, _ := governanceServices(pool)

	proposalID, changeID := submitTestProposal(t, ctx, proposalService, fixture)

	var auditCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM audit_events
		WHERE workspace_id = $1 AND event_type = 'governance.proposal.submitted'`,
		fixture.WorkspaceID.UUID()).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("governance audit rows = %d, want exactly one", auditCount)
	}

	var outboxID string
	var payload []byte
	if err := pool.QueryRow(ctx, `
		SELECT id::text, payload FROM outbox_events
		WHERE workspace_id = $1 AND event_type = 'proposal.changed'`,
		fixture.WorkspaceID.UUID()).Scan(&outboxID, &payload); err != nil {
		t.Fatalf("proposal.changed outbox row missing: %v", err)
	}
	raw, data := decodeOutboxEnvelope(t, payload)
	envelopeID, parseErr := identity.ParseEventID(string(bytes.Trim(raw["id"], `"`)))
	if parseErr != nil {
		t.Fatalf("envelope id %s is not an event TypeID: %v", raw["id"], parseErr)
	}
	if envelopeID.UUID() != outboxID {
		t.Fatalf("envelope id %s does not match outbox row id %s", envelopeID.UUID(), outboxID)
	}
	if data["specVersion"] != "semlia.proposal/v1" || data["action"] != "submitted" {
		t.Fatalf("event data = %v", data)
	}
	if data["fromState"] != "draft" || data["toState"] != "proposed" {
		t.Fatalf("event state transition = %v", data)
	}

	// The change-set froze on submission: the application refuses further edits
	// and the database trigger refuses raw SQL mutation (defense in depth).
	_, err := proposalService.AddChange(ctx, governanceapp.AddChangeRequest{
		WorkspaceID: fixture.WorkspaceID, ProposalID: proposalID,
		Item: governance.ChangeSetItem{FieldPath: "title", Op: governance.ChangeUpdate, AfterDigest: sha256FixtureDigest("x"), AfterValue: json.RawMessage(`"x"`)},
	})
	if !errors.Is(err, governance.ErrChangeSetFrozen) {
		t.Fatalf("add change after submit = %v, want ErrChangeSetFrozen", err)
	}
	if err := proposalService.RemoveChange(ctx, governanceapp.RemoveChangeRequest{
		WorkspaceID: fixture.WorkspaceID, ProposalID: proposalID, ChangeID: changeID,
	}); !errors.Is(err, governance.ErrChangeSetFrozen) {
		t.Fatalf("remove change after submit = %v, want ErrChangeSetFrozen", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE proposal_changes SET field_path = 'hacked' WHERE id = $1`, changeID.UUID()); err == nil {
		t.Fatal("database accepted change-set mutation after submission")
	}
	if _, err := pool.Exec(ctx, `
		UPDATE proposals SET state = 'released' WHERE id = $1`, proposalID.UUID()); err == nil {
		t.Fatal("database accepted proposed -> released state jump")
	}
}

func TestProposalStateWalkEmitsStateChangedEventsAndFreezesTerminalRows(t *testing.T) {
	resetSchema(t)
	pool := openPool(t)
	ctx := context.Background()
	fixture := seedGovernanceAsset(t, pool, "walk")
	proposalService, _, _, _, _ := governanceServices(pool)

	proposalID, _ := submitTestProposal(t, ctx, proposalService, fixture)
	if _, err := proposalService.Transition(ctx, governanceapp.TransitionRequest{
		WorkspaceID: fixture.WorkspaceID, ProposalID: proposalID,
		To: governance.ProposalValidating, Actor: "steward", TraceID: governanceTraceID,
	}); err != nil {
		t.Fatalf("proposed -> validating: %v", err)
	}
	if _, err := proposalService.Transition(ctx, governanceapp.TransitionRequest{
		WorkspaceID: fixture.WorkspaceID, ProposalID: proposalID,
		To: governance.ProposalInReview, Actor: "steward", TraceID: governanceTraceID,
	}); err != nil {
		t.Fatalf("validating -> in_review: %v", err)
	}
	// Packet red scenario: skipping straight to released is rejected in the
	// domain layer before any database write; released is reached only through
	// the ReleaseService cut transaction.
	if _, err := proposalService.Transition(ctx, governanceapp.TransitionRequest{
		WorkspaceID: fixture.WorkspaceID, ProposalID: proposalID,
		To: governance.ProposalReleased, Actor: "steward", TraceID: governanceTraceID,
	}); !errors.Is(err, governance.ErrInvalidTransition) {
		t.Fatalf("in_review -> released without release cut = %v, want ErrInvalidTransition", err)
	}

	var stateChanges int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM outbox_events
		WHERE workspace_id = $1 AND event_type = 'proposal.changed' AND payload->'data'->>'action' = 'state_changed'`,
		fixture.WorkspaceID.UUID()).Scan(&stateChanges); err != nil {
		t.Fatal(err)
	}
	if stateChanges != 2 {
		t.Fatalf("state_changed outbox events = %d, want 2", stateChanges)
	}

	if _, err := proposalService.Transition(ctx, governanceapp.TransitionRequest{
		WorkspaceID: fixture.WorkspaceID, ProposalID: proposalID,
		To: governance.ProposalRejected, Actor: "reviewer", TraceID: governanceTraceID,
	}); err != nil {
		t.Fatalf("in_review -> rejected: %v", err)
	}
	rejected, err := proposalService.GetProposal(ctx, fixture.WorkspaceID, proposalID)
	if err != nil {
		t.Fatal(err)
	}
	if !rejected.State.Terminal() || rejected.DecidedAt == nil {
		t.Fatalf("rejected proposal = %+v, want terminal with decided_at", rejected)
	}
	if _, err := proposalService.Transition(ctx, governanceapp.TransitionRequest{
		WorkspaceID: fixture.WorkspaceID, ProposalID: proposalID,
		To: governance.ProposalInReview, Actor: "reviewer", TraceID: governanceTraceID,
	}); !errors.Is(err, governance.ErrInvalidTransition) {
		t.Fatalf("rejected -> in_review = %v, want ErrInvalidTransition", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE proposals SET state = 'draft' WHERE id = $1`, proposalID.UUID()); err == nil {
		t.Fatal("database accepted terminal proposal state change")
	}
}

func TestSubmittedChangeSetsRejectConcurrentMutation(t *testing.T) {
	resetSchema(t)
	pool := openPool(t)
	ctx := context.Background()
	fixture := seedGovernanceAsset(t, pool, "concurrent")
	proposalService, _, _, _, _ := governanceServices(pool)
	proposalID, changeID := submitTestProposal(t, ctx, proposalService, fixture)
	_ = proposalID

	const attackers = 8
	concurrentPool := openConcurrentPool(t, attackers)
	t.Cleanup(concurrentPool.Close)
	start := make(chan struct{})
	var wg sync.WaitGroup
	failures := make([]error, attackers)
	for attacker := 0; attacker < attackers; attacker++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			if index%2 == 0 {
				_, err := concurrentPool.Exec(context.Background(), `
					UPDATE proposal_changes SET after_value = '"hacked"'::jsonb WHERE id = $1`, changeID.UUID())
				failures[index] = err
				return
			}
			_, err := concurrentPool.Exec(context.Background(), `
				DELETE FROM proposal_changes WHERE id = $1`, changeID.UUID())
			failures[index] = err
		}(attacker)
	}
	close(start)
	wg.Wait()
	for index, err := range failures {
		if err == nil {
			t.Fatalf("attacker %d mutated an immutable submitted change-set", index)
		}
	}
	var remaining int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM proposal_changes WHERE id = $1 AND field_path = 'definition.formula'`,
		changeID.UUID()).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 1 {
		t.Fatalf("submitted change rows after concurrent attack = %d, want 1 intact", remaining)
	}
}

func TestReleaseCutIsImmutableWithOutboxEventAndBlockerGate(t *testing.T) {
	resetSchema(t)
	pool := openPool(t)
	ctx := context.Background()
	fixture := seedGovernanceAsset(t, pool, "release")
	proposalService, validationService, _, releaseService, _ := governanceServices(pool)
	proposalID, _ := submitTestProposal(t, ctx, proposalService, fixture)

	if _, err := proposalService.Transition(ctx, governanceapp.TransitionRequest{
		WorkspaceID: fixture.WorkspaceID, ProposalID: proposalID,
		To: governance.ProposalValidating, Actor: "steward", TraceID: governanceTraceID,
	}); err != nil {
		t.Fatalf("start validation: %v", err)
	}

	// A Blocker validation result must block release candidacy (§3.3): the run
	// cannot be declared successful while its own findings contain a blocker,
	// and the release cut refuses while a reliable blocker finding exists.
	run, err := validationService.StartRun(ctx, governanceapp.StartRunRequest{
		WorkspaceID: fixture.WorkspaceID, ProposalID: proposalID,
		ValidatorID: "schema.lint", ValidatorVersion: "1.0.0",
	})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	if _, err := validationService.RecordResult(ctx, governanceapp.RecordResultRequest{
		WorkspaceID: fixture.WorkspaceID, ValidationRunID: run.ID,
		Severity: governance.SeverityBlocker, Code: "SCHEMA_BREAKING",
		Message: "removes a required dimension", InputDigest: sha256FixtureDigest("input"),
	}); err != nil {
		t.Fatalf("record blocker: %v", err)
	}
	if _, err := validationService.FinishRun(ctx, governanceapp.FinishRunRequest{
		WorkspaceID: fixture.WorkspaceID, RunID: run.ID, Status: governance.ValidationSucceeded,
	}); !errors.Is(err, governance.ErrInvariant) {
		t.Fatalf("succeeded run with blocker = %v, want ErrInvariant", err)
	}
	if _, err := releaseService.Cut(ctx, governanceapp.CutRequest{
		WorkspaceID: fixture.WorkspaceID, ProposalID: &proposalID,
		Entries:     []governance.ManifestEntry{{AssetID: fixture.AssetID, RevisionID: fixture.SecondRevID, Position: 1}},
		PublishedBy: "publisher", TraceID: governanceTraceID,
	}); !errors.Is(err, governance.ErrInvariant) {
		t.Fatalf("release with reliable blocker result = %v, want ErrInvariant", err)
	}
	if _, err := validationService.FinishRun(ctx, governanceapp.FinishRunRequest{
		WorkspaceID: fixture.WorkspaceID, RunID: run.ID, Status: governance.ValidationFailed,
	}); err != nil {
		t.Fatalf("mark run failed: %v", err)
	}

	// One run per proposal per validator version (data-model.md invariant).
	if _, err := validationService.StartRun(ctx, governanceapp.StartRunRequest{
		WorkspaceID: fixture.WorkspaceID, ProposalID: proposalID,
		ValidatorID: "schema.lint", ValidatorVersion: "1.0.0",
	}); !errors.Is(err, governance.ErrConflict) {
		t.Fatalf("duplicate validator version run = %v, want ErrConflict", err)
	}
	if _, err := validationService.StartRun(ctx, governanceapp.StartRunRequest{
		WorkspaceID: fixture.WorkspaceID, ProposalID: proposalID,
		ValidatorID: "schema.lint", ValidatorVersion: "1.1.0",
	}); err != nil {
		t.Fatalf("start clean run: %v", err)
	}
	if _, err := proposalService.Transition(ctx, governanceapp.TransitionRequest{
		WorkspaceID: fixture.WorkspaceID, ProposalID: proposalID,
		To: governance.ProposalInReview, Actor: "steward", TraceID: governanceTraceID,
	}); err != nil {
		t.Fatalf("validating -> in_review: %v", err)
	}

	entries := []governance.ManifestEntry{{
		AssetID: fixture.AssetID, RevisionID: fixture.SecondRevID, Position: 1,
		Compatibility: json.RawMessage(`{"conclusion":"compatible"}`),
	}}
	release, err := releaseService.Cut(ctx, governanceapp.CutRequest{
		WorkspaceID: fixture.WorkspaceID, ProposalID: &proposalID,
		Entries: entries, PublishedBy: "publisher", TraceID: governanceTraceID,
	})
	if err != nil {
		t.Fatalf("cut release: %v", err)
	}
	if release.State != governance.ReleasePublished || release.Sequence != 1 {
		t.Fatalf("cut release = %+v", release)
	}

	var storedDigest string
	if err := pool.QueryRow(ctx, `SELECT manifest_digest FROM releases WHERE id = $1`, release.ID.UUID()).Scan(&storedDigest); err != nil {
		t.Fatal(err)
	}
	manifestPayload, err := governance.ManifestDigestPayload(entries)
	if err != nil {
		t.Fatalf("canonicalize manifest: %v", err)
	}
	expectedDigest, err := governance.DigestJSON(manifestPayload)
	if err != nil {
		t.Fatalf("digest manifest: %v", err)
	}
	if storedDigest != expectedDigest {
		t.Fatalf("manifest digest = %s, want recomputable %s", storedDigest, expectedDigest)
	}

	var proposalState string
	if err := pool.QueryRow(ctx, `SELECT state FROM proposals WHERE id = $1`, proposalID.UUID()).Scan(&proposalState); err != nil {
		t.Fatal(err)
	}
	if proposalState != string(governance.ProposalReleased) {
		t.Fatalf("proposal state after release = %s, want released", proposalState)
	}

	var payload []byte
	if err := pool.QueryRow(ctx, `
		SELECT payload FROM outbox_events
		WHERE workspace_id = $1 AND event_type = 'release.published'`,
		fixture.WorkspaceID.UUID()).Scan(&payload); err != nil {
		t.Fatalf("release.published outbox row missing: %v", err)
	}
	_, data := decodeOutboxEnvelope(t, payload)
	if data["specVersion"] != "semlia.release/v1" || data["action"] != "published" {
		t.Fatalf("release event data = %v", data)
	}

	// Concurrent immutability attacks on the cut release and its manifest.
	const attackers = 6
	concurrentPool := openConcurrentPool(t, attackers)
	t.Cleanup(concurrentPool.Close)
	start := make(chan struct{})
	var wg sync.WaitGroup
	failures := make([]error, attackers)
	for attacker := 0; attacker < attackers; attacker++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			if index%2 == 0 {
				_, err := concurrentPool.Exec(context.Background(), `
					UPDATE releases SET manifest_digest = 'sha256:dead' WHERE id = $1`, release.ID.UUID())
				failures[index] = err
				return
			}
			_, err := concurrentPool.Exec(context.Background(), `
				DELETE FROM release_assets WHERE release_id = $1`, release.ID.UUID())
			failures[index] = err
		}(attacker)
	}
	close(start)
	wg.Wait()
	for index, err := range failures {
		if err == nil {
			t.Fatalf("attacker %d mutated an immutable release", index)
		}
	}
}

func TestReleaseRollbackCreatesNewReleaseReferencingTarget(t *testing.T) {
	resetSchema(t)
	pool := openPool(t)
	ctx := context.Background()
	fixture := seedGovernanceAsset(t, pool, "rollback")
	_, _, _, releaseService, _ := governanceServices(pool)

	first, cutErr := releaseService.Cut(ctx, governanceapp.CutRequest{
		WorkspaceID: fixture.WorkspaceID,
		Entries: []governance.ManifestEntry{{
			AssetID: fixture.AssetID, RevisionID: fixture.RevisionID, Position: 1,
		}},
		PublishedBy: "publisher", TraceID: governanceTraceID,
	})
	if cutErr != nil {
		t.Fatalf("cut first release: %v", cutErr)
	}
	second, err := releaseService.Cut(ctx, governanceapp.CutRequest{
		WorkspaceID: fixture.WorkspaceID,
		Entries: []governance.ManifestEntry{{
			AssetID: fixture.AssetID, RevisionID: fixture.SecondRevID, Position: 1,
		}},
		PublishedBy: "publisher", TraceID: governanceTraceID,
	})
	if err != nil {
		t.Fatalf("cut second release: %v", err)
	}
	if first.Sequence != 1 {
		t.Fatalf("first release sequence = %d, want 1", first.Sequence)
	}

	rollback, err := releaseService.Rollback(ctx, governanceapp.RollbackRequest{
		WorkspaceID: fixture.WorkspaceID, TargetReleaseID: second.ID,
		PublishedBy: "publisher", TraceID: governanceTraceID,
	})
	if err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if rollback.Sequence != 3 {
		t.Fatalf("rollback release sequence = %d, want 3 (rollback is a new release row)", rollback.Sequence)
	}
	if rollback.RolledBackToReleaseID == nil || *rollback.RolledBackToReleaseID != second.ID {
		t.Fatalf("rollback reference = %v, want target %s", rollback.RolledBackToReleaseID, second.ID)
	}
	entries, err := releaseService.ListReleaseAssets(ctx, fixture.WorkspaceID, rollback.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].RevisionID != fixture.RevisionID {
		t.Fatalf("rollback manifest = %+v, want the prior stable manifest restored", entries)
	}

	// The rolled-back target rows were never mutated or deleted.
	var targetDigest string
	if err := pool.QueryRow(ctx, `
		SELECT manifest_digest FROM releases WHERE id = $1`, second.ID.UUID()).Scan(&targetDigest); err != nil {
		t.Fatal(err)
	}
	if targetDigest != second.ManifestDigest {
		t.Fatalf("target release digest changed: %s != %s", targetDigest, second.ManifestDigest)
	}
	if _, err := releaseService.Rollback(ctx, governanceapp.RollbackRequest{
		WorkspaceID: fixture.WorkspaceID, TargetReleaseID: second.ID,
		PublishedBy: "publisher", TraceID: governanceTraceID,
	}); !errors.Is(err, governance.ErrConflict) {
		t.Fatalf("second rollback of the same target = %v, want ErrConflict (one rollback per release)", err)
	}
}

func TestPolicyDecisionsAreRecomputableAndImmutable(t *testing.T) {
	resetSchema(t)
	pool := openPool(t)
	ctx := context.Background()
	fixture := seedGovernanceAsset(t, pool, "policy")
	proposalService, _, policyService, _, _ := governanceServices(pool)
	proposalID, _ := submitTestProposal(t, ctx, proposalService, fixture)

	decision, err := policyService.Decide(ctx, governanceapp.DecideRequest{
		WorkspaceID: fixture.WorkspaceID, ProposalID: proposalID, RuleVersion: governance.RiskRuleVersion,
		Inputs: json.RawMessage(`{"assetType":"metric","affectsComputation":true,"blockerCount":0,"evidenceComplete":true}`),
	})
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	if decision.RiskLevel != governance.RiskMedium || decision.Routing != governance.RoutingExpert {
		t.Fatalf("computation-only change routed as %s/%s, want medium/expert", decision.RiskLevel, decision.Routing)
	}
	if decision.InputsDigest != sha256DigestOf(t, decision.Inputs) {
		t.Fatalf("inputs digest %s does not match the stored canonical inputs", decision.InputsDigest)
	}

	var riskLevel string
	if err := pool.QueryRow(ctx, `
		SELECT risk_level FROM proposals WHERE id = $1`, proposalID.UUID()).Scan(&riskLevel); err != nil {
		t.Fatal(err)
	}
	if riskLevel != string(governance.RiskMedium) {
		t.Fatalf("proposal risk level = %s, want medium", riskLevel)
	}
	if _, err := pool.Exec(ctx, `UPDATE policy_decisions SET risk_level = 'low' WHERE id = $1`, decision.ID.UUID()); err == nil {
		t.Fatal("database accepted policy decision mutation")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM policy_decisions WHERE id = $1`, decision.ID.UUID()); err == nil {
		t.Fatal("database accepted policy decision deletion")
	}
}

func TestAgentRunsRecordHashesDigestsCostDurationAndFinalState(t *testing.T) {
	resetSchema(t)
	pool := openPool(t)
	ctx := context.Background()
	fixture := seedGovernanceAsset(t, pool, "agent")
	_, _, _, _, agentRunService := governanceServices(pool)

	run, err := agentRunService.Start(ctx, governanceapp.StartAgentRunRequest{
		WorkspaceID: fixture.WorkspaceID, Model: "glm-5.3-flash", ConfigRevision: "cfg-2026-09-02",
		InputHash: sha256FixtureDigest("canonical-input"),
	})
	if err != nil {
		t.Fatalf("start agent run: %v", err)
	}
	step, err := agentRunService.RecordStep(ctx, governanceapp.RecordStepRequest{
		WorkspaceID: fixture.WorkspaceID, AgentRunID: run.ID, Sequence: 1, Kind: governance.AgentStepTool,
		ToolName: "catalog.search", InputHash: sha256FixtureDigest("tool-in"), OutputHash: sha256FixtureDigest("tool-out"),
	})
	if err != nil {
		t.Fatalf("record step: %v", err)
	}
	if step.ID.IsZero() {
		t.Fatal("step ID missing")
	}
	if _, err := agentRunService.RecordStep(ctx, governanceapp.RecordStepRequest{
		WorkspaceID: fixture.WorkspaceID, AgentRunID: run.ID, Sequence: 2, Kind: governance.AgentStepTool,
		InputHash: sha256FixtureDigest("tool-in"), OutputHash: sha256FixtureDigest("tool-out"),
	}); !errors.Is(err, governance.ErrInvalidArgument) {
		t.Fatalf("tool step without tool name = %v, want ErrInvalidArgument", err)
	}

	finished, err := agentRunService.Finish(ctx, governanceapp.FinishAgentRunRequest{
		WorkspaceID: fixture.WorkspaceID, RunID: run.ID, FinalState: governance.AgentRunSucceeded,
		OutputDigest: sha256FixtureDigest("final-output"), CostMicros: 1500,
	})
	if err != nil {
		t.Fatalf("finish agent run: %v", err)
	}
	if finished.Status != governance.AgentRunSucceeded || finished.OutputDigest == nil || finished.DurationMS == nil {
		t.Fatalf("finished run = %+v", finished)
	}
	if _, err := agentRunService.RecordStep(ctx, governanceapp.RecordStepRequest{
		WorkspaceID: fixture.WorkspaceID, AgentRunID: run.ID, Sequence: 3, Kind: governance.AgentStepModel,
		InputHash: sha256FixtureDigest("late"), OutputHash: sha256FixtureDigest("late"),
	}); !errors.Is(err, governance.ErrConflict) {
		t.Fatalf("step on finished run = %v, want ErrConflict", err)
	}
	if _, err := agentRunService.Finish(ctx, governanceapp.FinishAgentRunRequest{
		WorkspaceID: fixture.WorkspaceID, RunID: run.ID, FinalState: governance.AgentRunFailed,
	}); !errors.Is(err, governance.ErrConflict) {
		t.Fatalf("second finish = %v, want ErrConflict (final state is terminal)", err)
	}

	var model, configRevision, inputHash, status string
	var rawPromptLeak int
	if err := pool.QueryRow(ctx, `
		SELECT model, config_revision, input_hash, status FROM agent_runs WHERE id = $1`,
		run.ID.UUID()).Scan(&model, &configRevision, &inputHash, &status); err != nil {
		t.Fatal(err)
	}
	if model != "glm-5.3-flash" || configRevision != "cfg-2026-09-02" || status != "succeeded" {
		t.Fatalf("agent run row = model %s config %s status %s", model, configRevision, status)
	}
	if len(inputHash) != 71 || inputHash[:7] != "sha256:" {
		t.Fatalf("input hash %q is not a content digest", inputHash)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM agent_runs WHERE row_to_json(agent_runs)::text ILIKE '%prompt%'
		   OR row_to_json(agent_runs)::text ILIKE '%api_key%'`).Scan(&rawPromptLeak); err != nil {
		t.Fatal(err)
	}
	if rawPromptLeak != 0 {
		t.Fatal("agent run rows must never carry raw prompts or credentials")
	}
}

func openConcurrentPool(t *testing.T, connections int) *pgstore.Pool {
	t.Helper()
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	config.MaxConns = int32(connections)
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	return pool
}

func sha256DigestOf(t *testing.T, value json.RawMessage) string {
	t.Helper()
	canonical, err := governance.CanonicalJSON(value)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := governance.DigestJSON(canonical)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}
