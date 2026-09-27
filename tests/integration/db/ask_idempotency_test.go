package db_test

import (
	"context"
	"strings"
	"testing"
	"time"

	pgstore "github.com/iiwish/semlia/internal/adapters/postgres"
	app "github.com/iiwish/semlia/internal/application/governance"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

func TestAskMigrationPreserves32AndRefusesHistoryDowngrade(t *testing.T) {
	pool, migrator := semanticProductionDatabase(t, "ask_migration_33", 32)
	ctx := context.Background()
	w, _ := identity.NewWorkspaceID()
	if _, err := pool.Exec(ctx, `INSERT INTO workspaces(id,slug,display_name) VALUES($1,'ask-migration','Synthetic migration fixture')`, w.UUID()); err != nil {
		t.Fatal(err)
	}
	store := pgstore.NewStore(pool)
	agent, err := store.WorkspaceAgentPrincipal(ctx, w)
	if err != nil {
		t.Fatal(err)
	}
	actor, err := store.LoadDefaultPrincipal(ctx, w)
	if err != nil {
		t.Fatal(err)
	}
	digest := "sha256:" + strings.Repeat("a", 64)
	legacy, err := app.NewAgentRunService(store, app.ClockFunc(time.Now)).Start(ctx, app.StartAgentRunRequest{WorkspaceID: w, PrincipalID: &agent.ID, Model: "legacy-fixture", ConfigRevision: digest, InputHash: digest})
	if err != nil {
		t.Fatal(err)
	}
	if err := migrator.Steps(1); err != nil {
		t.Fatal(err)
	}
	assertVersion(t, migrator, 33, true)
	stored, err := store.GetAgentRun(ctx, w, legacy.ID)
	if err != nil || stored.InputHash != legacy.InputHash || stored.Model != legacy.Model {
		t.Fatalf("schema32 attribution changed: %v", err)
	}
	var forbidden int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND table_name='ask_requests' AND (data_type IN ('json','jsonb','bytea') OR column_name IN ('question','prompt','response','clarification','error_detail'))`).Scan(&forbidden); err != nil || forbidden != 0 {
		t.Fatalf("payload columns=%d err=%v", forbidden, err)
	}
	if err := migrator.Steps(-1); err != nil {
		t.Fatal(err)
	}
	assertVersion(t, migrator, 32, true)
	if err := migrator.Steps(1); err != nil {
		t.Fatal(err)
	}
	runID, _ := identity.NewAgentRunID()
	token, _ := identity.NewRunID()
	now := time.Now().UTC()
	record := domain.AskRequestRecord{WorkspaceID: w, RequestedBy: actor.ID, Key: "migration-fixture", InputDigest: digest, KnowledgeDigest: digest, RunID: runID, ClaimToken: token.UUID()}
	_, owned, err := store.ClaimAskRequest(ctx, record, domain.AgentRun{ID: runID, WorkspaceID: w, PrincipalID: &agent.ID, Model: "test", ConfigRevision: digest, InputHash: digest, Status: domain.AgentRunRunning, StartedAt: now, CreatedAt: now})
	if err != nil || !owned {
		t.Fatalf("claim=%v owned=%v", err, owned)
	}
	if err := migrator.Steps(-1); err == nil || !strings.Contains(err.Error(), "Ask history requires") {
		t.Fatalf("populated downgrade=%v", err)
	}
	if _, err := store.GetAskRequest(ctx, w, actor.ID, record.Key); err != nil {
		t.Fatalf("failed downgrade lost claim: %v", err)
	}
}
