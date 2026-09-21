package governance

import (
	"testing"

	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

func TestAutoDraftAssetAddress(t *testing.T) {
	cases := []struct {
		name, qualifiedName, want string
	}{
		{"schema-qualified", "public.zmm_gp_ttt", "public.zmm_gp_ttt"},
		{"unsafe-characters-sanitized", "public.订单 表!", "public._____"},
		{"bare-name-falls-under-semantic", "orders", "semantic.orders"},
		{"empty-name", "", "semantic.untitled"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := autoDraftAssetAddress(testCase.qualifiedName); got != testCase.want {
				t.Fatalf("autoDraftAssetAddress(%q) = %q, want %q", testCase.qualifiedName, got, testCase.want)
			}
		})
	}
}

func TestMatchingAutoDraftGrant(t *testing.T) {
	workspace := identity.WorkspaceID{}
	principal := identity.PrincipalID{}
	setting := identity.ModelSettingID{}
	revision := "sha256:20e2a3d3366370e834825628847c9239c2f3af54220ce744b9417b6b759f0ced"
	grant := domain.ProductionGenerationGrant{WorkspaceID: workspace, PrincipalID: principal,
		ModelSettingID: setting, ModelConfigRevision: revision, PricingBasis: "dev",
		MaxOutputTokens: 8192, MaxCostMicros: 50_000_000, MaxInputBytes: 262_144,
		InputMicrosPerByte: 1, OutputMicrosPerToken: 2000}
	if _, ok := matchingAutoDraftGrant([]domain.ProductionGenerationGrant{grant}, workspace, principal, setting, revision); !ok {
		t.Fatal("expected the exact grant to match")
	}
	stale := grant
	stale.ModelConfigRevision = "sha256:6d162cf55a07d92a6e157d322c9ec373f9295686906da3eaa47945e4f479bce9"
	if _, ok := matchingAutoDraftGrant([]domain.ProductionGenerationGrant{stale}, workspace, principal, setting, revision); ok {
		t.Fatal("a grant pinned to another config revision must not match")
	}
	unpriced := grant
	unpriced.PricingBasis = " "
	if _, ok := matchingAutoDraftGrant([]domain.ProductionGenerationGrant{unpriced}, workspace, principal, setting, revision); ok {
		t.Fatal("a grant without a pricing basis must not match")
	}
}

func TestCandidateAutoDraftConfigEnabled(t *testing.T) {
	if (CandidateAutoDraftConfig{}).enabled() {
		t.Fatal("empty config must disable the batch")
	}
	if (CandidateAutoDraftConfig{SchemaWhitelist: []string{"public"}, Limit: 0}).enabled() {
		t.Fatal("zero limit must disable the batch")
	}
	if !(CandidateAutoDraftConfig{SchemaWhitelist: []string{"public"}, Limit: 500}).enabled() {
		t.Fatal("whitelist plus positive limit must enable the batch")
	}
}
