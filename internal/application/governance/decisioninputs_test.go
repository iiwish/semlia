package governance_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	governanceapp "github.com/iiwish/semlia/internal/application/governance"
	"github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

func changeItemForClassification(fieldPath string) governance.ChangeSetItem {
	return governance.ChangeSetItem{
		FieldPath: fieldPath, Op: governance.ChangeAdd,
		AfterDigest: "sha256:" + strings.Repeat("a", 64), AfterValue: json.RawMessage(`"x"`),
	}
}

func mustInputTestID[T any](t *testing.T, create func() (T, error)) T {
	t.Helper()
	value, err := create()
	if err != nil {
		t.Fatal(err)
	}
	return value
}

// seededRulesV1 loads the canonical rule table the migration seeds; the
// integration suite locks the database rows to these values.
func seededRulesV1(t *testing.T) []governance.PolicyRule {
	t.Helper()
	rules, err := governance.CanonicalPolicyRules(governance.RiskRuleVersion)
	if err != nil {
		t.Fatalf("canonical rule table: %v", err)
	}
	return rules
}

func TestClassifyDiffCategoriesIsDeterministicByFieldPathPrefix(t *testing.T) {
	cases := []struct {
		fieldPath   string
		computation bool
		definition  bool
		relations   bool
		access      bool
		contract    bool
	}{
		{"expression", true, false, false, false, false},
		{"GrainExpression", true, false, false, false, false},
		{"join.cardinality", true, false, false, false, false},
		{"transform", true, false, false, false, false},
		{"uniquenessSemantics", true, false, false, false, false},
		{"definition", false, true, false, false, false},
		{"name", false, true, false, false, false},
		{"content.owner", false, true, false, false, false},
		{"documentation.summary", false, true, false, false, false},
		{"relations.depends_on", false, false, true, false, false},
		{"derived_from", false, false, true, false, false},
		{"permissions.read", false, false, false, true, false},
		{"access_scope", false, false, false, true, false},
		{"contract.compatibility", false, false, false, false, true},
		{"consumerNotice", false, false, false, false, true},
		{"genuinely-unknown-path", false, true, false, false, false},
	}
	for _, testCase := range cases {
		categories := governanceapp.ClassifyDiffCategories([]governance.ChangeSetItem{changeItemForClassification(testCase.fieldPath)})
		got := []bool{categories.Computation, categories.Definition, categories.Relations, categories.Access, categories.Contract}
		want := []bool{testCase.computation, testCase.definition, testCase.relations, testCase.access, testCase.contract}
		for index := range got {
			if got[index] != want[index] {
				t.Fatalf("field path %q category %d = %t, want %t (categories %+v)",
					testCase.fieldPath, index, got[index], want[index], categories)
			}
		}
		repeated := governanceapp.ClassifyDiffCategories([]governance.ChangeSetItem{changeItemForClassification(testCase.fieldPath)})
		if repeated != categories {
			t.Fatalf("classification of %q is not deterministic: %+v vs %+v", testCase.fieldPath, repeated, categories)
		}
	}
}

func fixedValidationOutcomes() governanceapp.ProposalValidationOutcomes {
	return governanceapp.ProposalValidationOutcomes{
		RunCount: 3, FailedCount: 0, BlockerCount: 0, WarningCount: 1, InfoCount: 3,
	}
}

func TestBuildDecisionInputsIsByteIdenticalForIdenticalPersistedState(t *testing.T) {
	// Packet red scenario: identical persisted state yields byte-identical
	// DecisionInputs and inputs_digest.
	proposal := governance.Proposal{
		ID: newGovernanceProposalID(t), TargetObjectType: governance.TargetSemanticAsset,
		State: governance.ProposalInReview, CreatedBy: "founder",
	}
	changes := []governance.ChangeSetItem{changeItemForClassification("definition")}
	target := governanceapp.TargetInputs{
		AssetType: "metric", EvidenceLinkCount: 2,
		OwnerContent: json.RawMessage(`{"owner":"revenue group"}`),
	}
	first, err := governanceapp.BuildDecisionInputs(proposal, changes, fixedValidationOutcomes(), target)
	if err != nil {
		t.Fatalf("first build: %v", err)
	}
	second, err := governanceapp.BuildDecisionInputs(proposal, changes, fixedValidationOutcomes(), target)
	if err != nil {
		t.Fatalf("second build: %v", err)
	}
	firstBytes, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	secondBytes, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstBytes) != string(secondBytes) {
		t.Fatalf("inputs are not byte-identical:\n%s\n%s", firstBytes, secondBytes)
	}
	firstDigest, err := governance.DigestJSON(firstBytes)
	if err != nil {
		t.Fatal(err)
	}
	secondDigest, err := governance.DigestJSON(secondBytes)
	if err != nil {
		t.Fatal(err)
	}
	if firstDigest != secondDigest {
		t.Fatalf("digests differ: %s != %s", firstDigest, secondDigest)
	}
}

func TestBuildDecisionInputsRepresentsClosedPersistedState(t *testing.T) {
	agentRunID := mustInputTestID(t, identity.NewAgentRunID)
	proposal := governance.Proposal{
		ID: newGovernanceProposalID(t), TargetObjectType: governance.TargetSemanticAsset,
		State: governance.ProposalInReview, AgentRunID: &agentRunID, CreatedBy: "agent-run:x",
	}
	changes := []governance.ChangeSetItem{
		changeItemForClassification("expression"), changeItemForClassification("permissions.read"),
	}
	inputs, err := governanceapp.BuildDecisionInputs(proposal, changes, governanceapp.ProposalValidationOutcomes{
		RunCount: 3, FailedCount: 1, BlockerCount: 1, WarningCount: 2, InfoCount: 4,
	}, governanceapp.TargetInputs{AssetType: "metric", EvidenceLinkCount: 0, OwnerContent: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !inputs.AffectsComputation || !inputs.AffectsAccess || inputs.AffectsDefinition || inputs.AffectsRelations || inputs.AffectsContract {
		t.Fatalf("diff categories = %+v, want computation+access only", inputs)
	}
	if inputs.AuthorKind != governance.AuthorKindAgent {
		t.Fatalf("author kind = %q, want agent", inputs.AuthorKind)
	}
	if inputs.OwnerAssigned != governance.OwnerUnassigned {
		t.Fatalf("owner assigned = %q, want unassigned", inputs.OwnerAssigned)
	}
	if inputs.TargetObjectType != "semantic_asset" || inputs.AssetType != "metric" {
		t.Fatalf("target/asset type = %q/%q", inputs.TargetObjectType, inputs.AssetType)
	}
	if inputs.ValidationRunCount != 3 || inputs.ValidationFailedCount != 1 || inputs.BlockerCount != 1 ||
		inputs.WarningCount != 2 || inputs.InfoCount != 4 {
		t.Fatalf("validation outcomes = %+v", inputs)
	}
	if inputs.EvidenceLinkCount != 0 || inputs.EvidenceComplete {
		t.Fatalf("evidence inputs = %d/%t, want 0/false", inputs.EvidenceLinkCount, inputs.EvidenceComplete)
	}
	if inputs.ConsumerCount != nil || inputs.HistoricalAcceptRate != nil {
		t.Fatalf("M4 placeholders must be not_applicable: %+v", inputs)
	}
	if inputs.ProductionEnvironment {
		t.Fatal("no persisted production-environment dimension exists in M2")
	}
	if err := inputs.Validate(); err != nil {
		t.Fatalf("built inputs must validate: %v", err)
	}
}

func TestBuildDecisionInputsOwnerStates(t *testing.T) {
	proposal := governance.Proposal{
		ID: newGovernanceProposalID(t), TargetObjectType: governance.TargetSemanticAsset,
		State: governance.ProposalInReview, CreatedBy: "founder",
	}
	cases := []struct {
		name  string
		owner json.RawMessage
		want  string
	}{
		{"assigned owner", json.RawMessage(`{"owner":" revenue group "}`), governance.OwnerAssigned},
		{"blank owner", json.RawMessage(`{"owner":"   "}`), governance.OwnerUnassigned},
		{"absent owner", json.RawMessage(`{"name":"x"}`), governance.OwnerUnassigned},
		{"non-string owner", json.RawMessage(`{"owner":42}`), governance.OwnerUnassigned},
		{"no content", nil, governance.OwnerNotApplicable},
	}
	for _, testCase := range cases {
		inputs, err := governanceapp.BuildDecisionInputs(proposal, nil, governanceapp.ProposalValidationOutcomes{},
			governanceapp.TargetInputs{AssetType: "metric", OwnerContent: testCase.owner})
		if err != nil {
			t.Fatalf("%s: build: %v", testCase.name, err)
		}
		if inputs.OwnerAssigned != testCase.want {
			t.Fatalf("%s: ownerAssigned = %q, want %q", testCase.name, inputs.OwnerAssigned, testCase.want)
		}
	}
}

// fakeDecisionFactsRepository serves the per-target input collectors with
// deterministic persisted facts.
type fakeDecisionFactsRepository struct {
	proposals    map[string]governance.Proposal
	changes      map[string][]governance.ChangeSetItem
	assetFacts   map[string]governanceapp.PolicyAssetFacts
	objectAssets map[string]identity.AssetID
	ownerContent map[string]json.RawMessage
	factsCalls   int
}

func (repository *fakeDecisionFactsRepository) GetProposal(
	_ context.Context, _ identity.WorkspaceID, proposal identity.ProposalID,
) (governance.Proposal, error) {
	stored, ok := repository.proposals[proposal.String()]
	if !ok {
		return governance.Proposal{}, governance.ErrNotFound
	}
	return stored, nil
}

func (repository *fakeDecisionFactsRepository) ListProposalChanges(
	_ context.Context, _ identity.WorkspaceID, proposal identity.ProposalID,
) ([]governance.ChangeSetItem, error) {
	return append([]governance.ChangeSetItem(nil), repository.changes[proposal.String()]...), nil
}

func (repository *fakeDecisionFactsRepository) SummarizeProposalValidationOutcomes(
	_ context.Context, _ identity.WorkspaceID, _ identity.ProposalID,
) (governanceapp.ProposalValidationOutcomes, error) {
	return fixedValidationOutcomes(), nil
}

func (repository *fakeDecisionFactsRepository) GetPolicyAssetFacts(
	_ context.Context, _ identity.WorkspaceID, asset identity.AssetID,
) (governanceapp.PolicyAssetFacts, error) {
	repository.factsCalls++
	facts, ok := repository.assetFacts[asset.String()]
	if !ok {
		return governanceapp.PolicyAssetFacts{}, governance.ErrNotFound
	}
	return facts, nil
}

func (repository *fakeDecisionFactsRepository) GetPolicyGovernedObjectAsset(
	_ context.Context, _ identity.WorkspaceID, objectUUID string,
) (identity.AssetID, error) {
	asset, ok := repository.objectAssets[objectUUID]
	if !ok {
		return identity.AssetID{}, governance.ErrNotFound
	}
	return asset, nil
}

func (repository *fakeDecisionFactsRepository) GetPolicyRevisionOwnerContent(
	_ context.Context, _ identity.WorkspaceID, asset identity.AssetID, _ identity.RevisionID,
) (json.RawMessage, error) {
	content, ok := repository.ownerContent[asset.String()]
	if !ok {
		return nil, governance.ErrNotFound
	}
	return content, nil
}

func newCollectorFixture(t *testing.T) (*fakeDecisionFactsRepository, *governanceapp.InputsCollectorRegistry) {
	t.Helper()
	facts := &fakeDecisionFactsRepository{
		proposals: map[string]governance.Proposal{}, changes: map[string][]governance.ChangeSetItem{},
		assetFacts: map[string]governanceapp.PolicyAssetFacts{}, objectAssets: map[string]identity.AssetID{},
		ownerContent: map[string]json.RawMessage{},
	}
	return facts, governanceapp.NewInputsCollectorRegistry(facts)
}

func TestInputsCollectorRegistryAddsCollectorsPerTargetType(t *testing.T) {
	_, registry := newCollectorFixture(t)
	for _, targetType := range []governance.TargetObjectType{
		governance.TargetSemanticAsset, governance.TargetPhysicalBinding, governance.TargetModelGrain,
		governance.TargetEntityKey, governance.TargetJoinContract,
	} {
		if _, err := registry.For(targetType); err != nil {
			t.Fatalf("collector for %s: %v", targetType, err)
		}
	}
	if _, err := registry.For(governance.TargetObjectType("dashboard")); !errors.Is(err, governance.ErrInvalidArgument) {
		t.Fatalf("unknown target error = %v, want ErrInvalidArgument", err)
	}
}

func mustCollectorFor(t *testing.T, registry *governanceapp.InputsCollectorRegistry, targetType governance.TargetObjectType) governanceapp.TargetInputsCollector {
	t.Helper()
	collector, err := registry.For(targetType)
	if err != nil {
		t.Fatalf("collector for %s: %v", targetType, err)
	}
	return collector
}

func TestInputsCollectorResolvesSemanticAssetTargetFromBaseRevision(t *testing.T) {
	facts, registry := newCollectorFixture(t)
	assetID := mustInputTestID(t, identity.NewAssetID)
	revisionID := mustInputTestID(t, identity.NewRevisionID)
	facts.assetFacts[assetID.String()] = governanceapp.PolicyAssetFacts{
		AssetType: "metric", EvidenceLinkCount: 4, CurrentRevisionContent: json.RawMessage(`{"owner":"current"}`),
	}
	facts.ownerContent[assetID.String()] = json.RawMessage(`{"owner":"base"}`)
	proposal := governance.Proposal{
		ID: newGovernanceProposalID(t), WorkspaceID: newGovernanceWorkspaceID(t),
		TargetObjectType: governance.TargetSemanticAsset, TargetObjectID: assetID.UUID(),
		AssetID: &assetID, BaseRevisionID: &revisionID,
	}
	target, err := mustCollectorFor(t, registry, governance.TargetSemanticAsset).CollectTargetInputs(context.Background(), proposal.WorkspaceID, proposal)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if target.AssetType != "metric" || target.EvidenceLinkCount != 4 {
		t.Fatalf("target inputs = %+v", target)
	}
	if string(target.OwnerContent) != `{"owner":"base"}` {
		t.Fatalf("semantic-asset owner must come from the base revision: %s", target.OwnerContent)
	}
}

func TestInputsCollectorResolvesAssetScopedGovernedObjects(t *testing.T) {
	facts, registry := newCollectorFixture(t)
	assetID := mustInputTestID(t, identity.NewAssetID)
	objectID := mustInputTestID(t, identity.NewModelGrainID)
	facts.objectAssets[objectID.UUID()] = assetID
	facts.assetFacts[assetID.String()] = governanceapp.PolicyAssetFacts{
		AssetType: "entity", EvidenceLinkCount: 1, CurrentRevisionContent: json.RawMessage(`{"owner":"stewards"}`),
	}
	proposal := governance.Proposal{
		ID: newGovernanceProposalID(t), WorkspaceID: newGovernanceWorkspaceID(t),
		TargetObjectType: governance.TargetModelGrain, TargetObjectID: objectID.UUID(),
	}
	target, err := mustCollectorFor(t, registry, governance.TargetModelGrain).CollectTargetInputs(context.Background(), proposal.WorkspaceID, proposal)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if target.AssetType != "entity" || target.EvidenceLinkCount != 1 || string(target.OwnerContent) != `{"owner":"stewards"}` {
		t.Fatalf("target inputs = %+v", target)
	}
}

func TestInputsCollectorMarksJoinContractNotApplicable(t *testing.T) {
	facts, registry := newCollectorFixture(t)
	contractID := mustInputTestID(t, identity.NewJoinContractID)
	proposal := governance.Proposal{
		ID: newGovernanceProposalID(t), WorkspaceID: newGovernanceWorkspaceID(t),
		TargetObjectType: governance.TargetJoinContract, TargetObjectID: contractID.UUID(),
	}
	target, err := mustCollectorFor(t, registry, governance.TargetJoinContract).CollectTargetInputs(context.Background(), proposal.WorkspaceID, proposal)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if target.AssetType != governance.DecisionInputNotApplicable || target.EvidenceLinkCount != 0 || target.OwnerContent != nil {
		t.Fatalf("join-contract target inputs = %+v, want not-applicable facts", target)
	}
	if facts.factsCalls != 0 {
		t.Fatal("join contracts have no asset facts to resolve")
	}
}

// conflictAwarePolicyRepository mirrors the storage contract of
// CreatePolicyDecision: the UNIQUE (proposal_id, rule_version, inputs_digest)
// index collapses duplicate inserts into the existing row.
type conflictAwarePolicyRepository struct {
	decisions []governance.PolicyDecision
}

func (repository *conflictAwarePolicyRepository) CreatePolicyDecision(
	_ context.Context, command governanceapp.PolicyDecisionCommand,
) (governance.PolicyDecision, error) {
	for _, decision := range repository.decisions {
		if decision.ProposalID == command.ProposalID && decision.RuleVersion == command.RuleVersion &&
			decision.InputsDigest == command.InputsDigest {
			return decision, nil
		}
	}
	created := governance.PolicyDecision{
		ID: command.ID, WorkspaceID: command.WorkspaceID, ProposalID: command.ProposalID,
		RuleVersion: command.RuleVersion, Inputs: command.Inputs, InputsDigest: command.InputsDigest,
		MatchedPolicy: command.MatchedPolicy, RiskLevel: command.RiskLevel,
		Routing: command.Routing, ReasonCode: command.ReasonCode, DecidedAt: command.DecidedAt,
	}
	repository.decisions = append(repository.decisions, created)
	return created, nil
}

func (repository *conflictAwarePolicyRepository) GetPolicyDecision(
	_ context.Context, _ identity.WorkspaceID, id identity.PolicyDecisionID,
) (governance.PolicyDecision, error) {
	for _, decision := range repository.decisions {
		if decision.ID == id {
			return decision, nil
		}
	}
	return governance.PolicyDecision{}, governance.ErrNotFound
}

func TestEnsureDecisionPersistsExactlyOneDecisionPerInputsDigest(t *testing.T) {
	repository := &conflictAwarePolicyRepository{}
	source := governanceapp.NewStaticRuleSource(governance.RiskRuleVersion, seededRulesV1(t))
	service := governanceapp.NewPolicyService(repository, governanceapp.ClockFunc(func() time.Time {
		return time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC)
	}), governanceapp.WithRuleSource(source))
	workspace := newGovernanceWorkspaceID(t)
	proposal := newGovernanceProposalID(t)
	request := governanceapp.EnsureDecisionRequest{
		WorkspaceID: workspace, ProposalID: proposal, RuleVersion: governance.RiskRuleVersion,
		Inputs: json.RawMessage(`{"assetType":"metric","targetObjectType":"semantic_asset","affectsComputation":true,"authorKind":"agent"}`),
	}
	first, err := service.EnsureDecision(context.Background(), request)
	if err != nil {
		t.Fatalf("first ensure: %v", err)
	}
	second, err := service.EnsureDecision(context.Background(), request)
	if err != nil {
		t.Fatalf("second ensure: %v", err)
	}
	if len(repository.decisions) != 1 {
		t.Fatalf("decision rows = %d, want exactly one", len(repository.decisions))
	}
	if first.ID != second.ID || first.InputsDigest != second.InputsDigest {
		t.Fatalf("ensure returned different decisions: %+v vs %+v", first, second)
	}
	if first.MatchedPolicy != "semlia.risk.v1/computation" || first.Routing != governance.RoutingExpert {
		t.Fatalf("decision = %+v, want the seeded computation rule outcome", first)
	}
}

// staticRuleProbeRepository records which rule source fed the decision.
type staticRuleProbeRepository struct {
	decisions []governance.PolicyDecision
}

func (repository *staticRuleProbeRepository) CreatePolicyDecision(
	_ context.Context, command governanceapp.PolicyDecisionCommand,
) (governance.PolicyDecision, error) {
	created := governance.PolicyDecision{
		ID: command.ID, WorkspaceID: command.WorkspaceID, ProposalID: command.ProposalID,
		RuleVersion: command.RuleVersion, Inputs: command.Inputs, InputsDigest: command.InputsDigest,
		MatchedPolicy: command.MatchedPolicy, RiskLevel: command.RiskLevel,
		Routing: command.Routing, ReasonCode: command.ReasonCode, DecidedAt: command.DecidedAt,
	}
	repository.decisions = append(repository.decisions, created)
	return created, nil
}

func (repository *staticRuleProbeRepository) GetPolicyDecision(
	_ context.Context, _ identity.WorkspaceID, _ identity.PolicyDecisionID,
) (governance.PolicyDecision, error) {
	return governance.PolicyDecision{}, governance.ErrNotFound
}

func TestPolicyServiceEvaluatesThroughTheRuleSource(t *testing.T) {
	repository := &staticRuleProbeRepository{}
	source := governanceapp.NewStaticRuleSource("2.0", []governance.PolicyRule{{
		ID: "custom.v2/always-expert", RuleVersion: "2.0", Priority: 1, Match: json.RawMessage(`{}`),
		RiskLevel: governance.RiskHigh, Routing: governance.RoutingExpert,
		ReasonCode: "CUSTOM_EXPERT", Explanation: "test rules route everything to expert review",
	}})
	service := governanceapp.NewPolicyService(repository, governanceapp.ClockFunc(time.Now), governanceapp.WithRuleSource(source))
	decision, err := service.Decide(context.Background(), governanceapp.DecideRequest{
		WorkspaceID: newGovernanceWorkspaceID(t), ProposalID: newGovernanceProposalID(t), RuleVersion: "2.0",
		Inputs: json.RawMessage(`{"assetType":"concept","blockerCount":0}`),
	})
	if err != nil {
		t.Fatalf("decide with rule source: %v", err)
	}
	if decision.MatchedPolicy != "custom.v2/always-expert" || decision.ReasonCode != "CUSTOM_EXPERT" {
		t.Fatalf("decision = %+v, want the rule-source outcome", decision)
	}
}
