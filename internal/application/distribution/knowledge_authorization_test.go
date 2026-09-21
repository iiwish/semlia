package distribution_test

import (
	"context"
	"testing"

	distributionapp "github.com/iiwish/semlia/internal/application/distribution"
	"github.com/iiwish/semlia/internal/domain/distribution"
	"github.com/iiwish/semlia/internal/testsupport/knowledgecase"
	"github.com/iiwish/semlia/pkg/identity"
)

func TestModelSelectionIgnoresUnauthorizedModelAndSelectorCandidates(t *testing.T) {
	for _, test := range []string{"model", "selector"} {
		t.Run(test, func(t *testing.T) {
			c := knowledgecase.New()
			workspace := serviceID(t, identity.NewWorkspaceID)
			principal := serviceID(t, identity.NewPrincipalID)
			c.Snapshot.WorkspaceID = workspace
			original := c.Snapshot.Assets[len(c.Snapshot.Assets)-1]
			if test == "selector" {
				original = c.Snapshot.Assets[8]
				c.Query.Measures[0] = distribution.Selector{Search: original.Name}
			}
			denied := original
			denied.AssetID = serviceID(t, identity.NewAssetID)
			c.Snapshot.Assets = append(c.Snapshot.Assets, denied)
			repo := &fakeDistributionRepository{current: c.Snapshot}
			auth := &fakeDistributionAuthorizer{principal: principal, deniedAssets: map[string]bool{denied.AssetID.UUID(): true}}
			result, err := distributionapp.NewService(repo, auth, serviceClock).Resolve(context.Background(), distributionapp.ResolveRequest{
				WorkspaceID: workspace, Input: c.Query, Channel: "api", IdempotencyKey: "authorized-knowledge", PrincipalRef: principal.String(),
			})
			if err != nil || result.Refusal != nil || result.Plan == nil {
				t.Fatalf("unreadable candidate changed authorized resolution: %+v %v", result.Refusal, err)
			}
			for _, asset := range result.Plan.Assets {
				if asset.AssetID == denied.AssetID {
					t.Fatal("unreadable asset in plan")
				}
			}
		})
	}
}

func TestModelExpansionRefusesUnauthorizedDependencyWithoutPlan(t *testing.T) {
	c := knowledgecase.New()
	workspace := serviceID(t, identity.NewWorkspaceID)
	principal := serviceID(t, identity.NewPrincipalID)
	c.Snapshot.WorkspaceID = workspace
	denied, _ := identity.ParseAssetID(c.Refs["order_data"].AssetID)
	repo := &fakeDistributionRepository{current: c.Snapshot}
	auth := &fakeDistributionAuthorizer{principal: principal, deniedAssets: map[string]bool{denied.UUID(): true}}
	result, err := distributionapp.NewService(repo, auth, serviceClock).Resolve(context.Background(), distributionapp.ResolveRequest{
		WorkspaceID: workspace, Input: c.Query, Channel: "api", IdempotencyKey: "denied-dependency", PrincipalRef: principal.String(),
	})
	if err != nil || result.Refusal == nil || result.Plan != nil || len(result.Definitions) != 0 || len(result.Refusal.CandidateIDs) != 0 {
		t.Fatalf("unreadable dependency exposed a plan or definition: %+v %v", result, err)
	}
}
