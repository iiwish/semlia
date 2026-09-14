package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/iiwish/semlia/internal/application"
	authapp "github.com/iiwish/semlia/internal/application/authorization"
	governanceapp "github.com/iiwish/semlia/internal/application/governance"
	coredomain "github.com/iiwish/semlia/internal/domain"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	httpapi "github.com/iiwish/semlia/internal/platform/http"
	"github.com/iiwish/semlia/pkg/identity"
	"go.opentelemetry.io/otel/trace/noop"
)

// The page query observes v1; a replacement commits before per-item recovery.
type productionListRevisionRepo struct {
	*mockProductionRepo
	listed domain.ProductionOperation
}

func (r *productionListRevisionRepo) ListProductionOperationsPage(context.Context, identity.WorkspaceID, domain.ProductionListQuery) ([]domain.ProductionOperation, error) {
	return []domain.ProductionOperation{r.listed}, nil
}

func (r *productionListRevisionRepo) GetProductionOperationVersion(_ context.Context, _ identity.WorkspaceID, _ identity.ProductionOperationID, version int) (domain.ProductionOperation, domain.ProductionVersion, []domain.ProductionTarget, []domain.ProductionCandidateLink, []domain.ProductionContributor, error) {
	if version != 1 {
		return domain.ProductionOperation{}, domain.ProductionVersion{}, nil, nil, nil, domain.ErrNotFound
	}
	return r.listed, domain.ProductionVersion{Version: 1}, []domain.ProductionTarget{{LocalKey: "original"}}, nil, nil, nil
}

func TestProductionListKeepsSelectedVersionDuringReplacement(t *testing.T) {
	w, p := mustWorkspaceID(t), mustPrincipalID(t)
	opID, err := identity.NewProductionOperationID()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	op := domain.ProductionOperation{ID: opID, WorkspaceID: w, CreatedBy: p, CurrentVersion: 1, CreatedAt: now, UpdatedAt: now}
	r := &productionListRevisionRepo{mockProductionRepo: newMockProductionRepo(), listed: op}
	key := fmt.Sprintf("%s:%s", w, opID)
	op.CurrentVersion = 2
	r.operations[key] = op
	r.versions[key] = domain.ProductionVersion{Version: 2, FrozenAt: &now}
	r.targets[key] = []domain.ProductionTarget{{LocalKey: "new-one"}, {LocalKey: "new-two"}}
	authorizer := authapp.NewService(&productionHTTPAuthorizationRepository{workspace: w, principal: p}, authapp.ClockFunc(time.Now))
	h := httpapi.NewHandler(application.NewSystemService(nil, coredomain.SystemInfo{}), slog.New(slog.NewTextHandler(io.Discard, nil)), noop.NewTracerProvider().Tracer("test"), httpapi.WithProduction(governanceapp.NewProductionService(r)), httpapi.WithAuthorization(authorizer))
	request := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/workspaces/%s/production-operations", w), nil)
	request.Header.Set("X-Semlia-Principal", p.String())
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("list: %d %s", response.Code, response.Body.String())
	}
	var page struct {
		Items []struct {
			CurrentVersion int  `json:"currentVersion"`
			TargetCount    int  `json:"targetCount"`
			Frozen         bool `json:"frozen"`
		} `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].CurrentVersion != 1 || page.Items[0].TargetCount != 1 || page.Items[0].Frozen {
		t.Fatalf("list mixed selected and later version: %s", response.Body.String())
	}
}
