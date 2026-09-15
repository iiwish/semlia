package governance_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	authapp "github.com/iiwish/semlia/internal/application/authorization"
	governanceapp "github.com/iiwish/semlia/internal/application/governance"
	governance "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

type rollbackPublicationBarrier struct {
	governanceapp.PublishingRepository
	beforeRestore func()
}

func (repository rollbackPublicationBarrier) RestoreRelease(ctx context.Context, command governanceapp.RestoreReleaseCommand) (governance.Release, error) {
	repository.beforeRestore()
	return repository.PublishingRepository.RestoreRelease(ctx, command)
}

func TestExecutionRollbackRechecksConcurrentPublicationAfterPreflight(t *testing.T) {
	env := newFixture(t)
	w := createWorkspace(t, env.pool, "rollback-race")
	author := createPrincipalWithRoles(t, env, w, "author", []string{"asset_owner"})
	publisher := createPrincipalWithRoles(t, env, w, "publisher", []string{"publisher"})
	reviewer := createReviewerPrincipal(t, env, w, "reviewer")
	a, ar := env.createAssetWithAddress(t, w, "race.a")
	b, br := env.createAssetWithAddress(t, w, "race.b")
	_, _, ap := proposeToInReview(t, env, w, author, a, ar)
	approveProposal(t, env, w, reviewer, ap)
	first := publishProposal(t, env, w, publisher, ap)
	if first.Code != http.StatusCreated {
		t.Fatal("initial publication failed")
	}
	var firstRelease struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &firstRelease); err != nil {
		t.Fatal(err)
	}
	_, _, bp := proposeToInReview(t, env, w, author, b, br)
	approveProposal(t, env, w, reviewer, bp)
	barrier := rollbackPublicationBarrier{PublishingRepository: env.store, beforeRestore: func() {
		second := publishProposal(t, env, w, publisher, bp)
		if second.Code != http.StatusCreated {
			t.Fatal("concurrent publication failed")
		}
	}}
	service := governanceapp.NewPublishingService(barrier, authapp.NewService(env.store, authapp.ClockFunc(time.Now)), governanceapp.ClockFunc(time.Now))
	id, _ := identity.ParseReleaseID(firstRelease.ID)
	_, err := service.RollbackRelease(context.Background(), governanceapp.RollbackReleaseRequest{WorkspaceID: w, ReleaseID: id, PrincipalRef: publisher.String(), TraceID: traceID})
	if !errors.Is(err, governance.ErrConflict) {
		t.Fatalf("rollback must refuse newer publication: %v", err)
	}
	sequence, err := env.store.MaxReleaseSequence(context.Background(), w)
	if err != nil || sequence != 2 {
		t.Fatalf("concurrent publication lost: sequence=%d error=%v", sequence, err)
	}
	for _, asset := range []identity.AssetID{a, b} {
		detail, err := env.store.GetCatalogAsset(context.Background(), w, asset)
		if err != nil || detail.CurrentRevision == nil || detail.CurrentRevision.ID == ar || detail.CurrentRevision.ID == br {
			t.Fatal("rollback changed a published revision")
		}
	}
}
