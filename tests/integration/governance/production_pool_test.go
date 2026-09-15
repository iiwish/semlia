package governance_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	pgstore "github.com/iiwish/semlia/internal/adapters/postgres"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestProductionInputAuthorizationUsesTransactionConnection(t *testing.T) {
	f, _, workspace, author := authoringLifecycleSetup(t)
	config := f.pool.Config().Copy()
	config.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err = pgstore.NewStore(pool).CheckProductionInput(ctx, domain.ProductionVersion{
		WorkspaceID: workspace, CreatedBy: author, InputJSON: json.RawMessage(`{}`),
	}, nil)
	// Authorization must complete on the already-held connection before input
	// validation rejects the intentionally missing snapshot.
	if !errors.Is(err, domain.ErrInputIncomplete) {
		t.Fatalf("expected input validation, not a pool wait: %v", err)
	}
}

func TestProductionReadsAndActionsWithOneConnection(t *testing.T) {
	f, handler, workspace, author := authoringLifecycleSetup(t)
	input := productionFixtureInput(t, f, workspace, identity.SemanticCandidateID{})
	body := productionPayloadInput(t, authoringLifecycleBody(`{}`), input, author)
	response := sendProdRequest(handler, http.MethodPost, fmt.Sprintf("/api/v1/workspaces/%s/production-operations", workspace), author.String(), "pool-draft", body)
	if response.Code != http.StatusCreated {
		t.Fatalf("create draft: %d", response.Code)
	}
	created := authoringLifecycleDecode(t, response.Body.Bytes())
	operation := mustParseProductionOperationID(t, created["operationId"].(string))
	config := f.pool.Config().Copy()
	config.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	store := pgstore.NewStore(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, version, targets, _, _, err := store.GetProductionOperation(ctx, workspace, operation)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CheckProductionInput(ctx, version, targets); err != nil {
		t.Fatal(err)
	}
	if err := store.CheckProductionAction(ctx, version, targets, domain.CommandSubmit); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReadProductionBusinessRules(ctx, workspace, author, operation, 1); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.ReadProductionGenerationHistory(ctx, workspace, author, operation, 1); err != nil {
		t.Fatal(err)
	}
}

func TestProductionGenerationWithOneConnection(t *testing.T) {
	g := newProductionGenerationFixture(t)
	config := g.f.pool.Config().Copy()
	config.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	store := pgstore.NewStore(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := store.QueueProductionGeneration(ctx, g.request, []domain.ProductionGenerationGrant{g.grant}, "protocol_stub")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReadProductionGeneration(ctx, g.w, g.actor, g.op, result.RunID); err != nil {
		t.Fatal(err)
	}
}
