package governance_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/iiwish/semlia/internal/domain/governance"
)

func TestLegalTransitionTableWalksSSOT75Path(t *testing.T) {
	// The exact §7.5 proposal path: draft -> proposed -> validating ->
	// in_review -> released. Every step must be legal and land on the next
	// workflow state without collapsing any other status dimension into it.
	path := []governance.ProposalState{
		governance.ProposalDraft,
		governance.ProposalProposed,
		governance.ProposalValidating,
		governance.ProposalInReview,
		governance.ProposalReleased,
	}
	for index := 0; index < len(path)-1; index++ {
		if !governance.LegalProposalTransition(path[index], path[index+1]) {
			t.Fatalf("legal transition %s -> %s rejected", path[index], path[index+1])
		}
	}
}

func TestRejectionPathsAreLegalFromEveryNonTerminalState(t *testing.T) {
	for _, state := range []governance.ProposalState{
		governance.ProposalDraft,
		governance.ProposalProposed,
		governance.ProposalValidating,
		governance.ProposalInReview,
	} {
		if !governance.LegalProposalTransition(state, governance.ProposalRejected) {
			t.Fatalf("rejection from %s must be legal", state)
		}
	}
}

func TestIllegalTransitionsAreRejected(t *testing.T) {
	// Packet red scenario: skipping states and leaving terminal states is
	// rejected by the domain, never by the database alone.
	illegal := []struct {
		from governance.ProposalState
		to   governance.ProposalState
	}{
		{governance.ProposalDraft, governance.ProposalReleased},
		{governance.ProposalDraft, governance.ProposalValidating},
		{governance.ProposalDraft, governance.ProposalInReview},
		{governance.ProposalProposed, governance.ProposalReleased},
		{governance.ProposalProposed, governance.ProposalDraft},
		{governance.ProposalValidating, governance.ProposalProposed},
		{governance.ProposalInReview, governance.ProposalValidating},
		{governance.ProposalReleased, governance.ProposalRejected},
		{governance.ProposalReleased, governance.ProposalDraft},
		{governance.ProposalRejected, governance.ProposalInReview},
		{governance.ProposalRejected, governance.ProposalDraft},
	}
	for _, scenario := range illegal {
		err := governance.CheckProposalTransition(scenario.from, scenario.to)
		if !errors.Is(err, governance.ErrInvalidTransition) {
			t.Fatalf("transition %s -> %s error = %v, want ErrInvalidTransition", scenario.from, scenario.to, err)
		}
	}
}

func TestTerminalStatesAcceptNoOutgoingTransition(t *testing.T) {
	for _, state := range []governance.ProposalState{governance.ProposalReleased, governance.ProposalRejected} {
		if !state.Terminal() {
			t.Fatalf("%s must be terminal for the proposal aggregate", state)
		}
		if !governance.LegalProposalTransition(state, state) {
			t.Fatalf("self transition %s -> %s must stay legal (no-op)", state, state)
		}
	}
}

func TestProposalStateVocabularyIsClosed(t *testing.T) {
	for _, state := range []governance.ProposalState{
		governance.ProposalDraft, governance.ProposalProposed, governance.ProposalValidating,
		governance.ProposalInReview, governance.ProposalReleased, governance.ProposalRejected,
	} {
		if !state.Valid() {
			t.Fatalf("state %s must be part of the closed vocabulary", state)
		}
	}
	for _, state := range []governance.ProposalState{"", "deployed", "healthy", "deprecated", "retired"} {
		if state.Valid() {
			t.Fatalf("state %q must not be valid: deployment and health states are separate dimensions", state)
		}
	}
}

func TestChangeSetItemValidatesOpValueConsistency(t *testing.T) {
	digest := "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	cases := []struct {
		name    string
		item    governance.ChangeSetItem
		wantErr bool
	}{
		{
			name: "valid update",
			item: governance.ChangeSetItem{
				FieldPath: "definition.formula", Op: governance.ChangeUpdate,
				BeforeDigest: digest, AfterDigest: digest,
				BeforeValue: json.RawMessage(`"old"`), AfterValue: json.RawMessage(`"new"`),
			},
		},
		{
			name: "valid add",
			item: governance.ChangeSetItem{
				FieldPath: "aliases", Op: governance.ChangeAdd, AfterDigest: digest, AfterValue: json.RawMessage(`["x"]`),
			},
		},
		{
			name: "valid remove",
			item: governance.ChangeSetItem{
				FieldPath: "aliases", Op: governance.ChangeRemove, BeforeDigest: digest, BeforeValue: json.RawMessage(`["x"]`),
			},
		},
		{
			name:    "add cannot carry before value",
			item:    governance.ChangeSetItem{FieldPath: "aliases", Op: governance.ChangeAdd, BeforeDigest: digest, AfterValue: json.RawMessage(`[]`)},
			wantErr: true,
		},
		{
			name:    "remove cannot carry after value",
			item:    governance.ChangeSetItem{FieldPath: "aliases", Op: governance.ChangeRemove, BeforeValue: json.RawMessage(`[]`), AfterDigest: digest},
			wantErr: true,
		},
		{
			name:    "update requires both digests",
			item:    governance.ChangeSetItem{FieldPath: "a.b", Op: governance.ChangeUpdate, AfterDigest: digest, AfterValue: json.RawMessage(`1`)},
			wantErr: true,
		},
		{
			name:    "empty field path",
			item:    governance.ChangeSetItem{Op: governance.ChangeAdd, AfterDigest: digest, AfterValue: json.RawMessage(`1`)},
			wantErr: true,
		},
		{
			name:    "digest without value",
			item:    governance.ChangeSetItem{FieldPath: "a", Op: governance.ChangeAdd, AfterDigest: digest},
			wantErr: true,
		},
		{
			name:    "unknown op",
			item:    governance.ChangeSetItem{FieldPath: "a", Op: "replace", AfterDigest: digest, AfterValue: json.RawMessage(`1`)},
			wantErr: true,
		},
		{
			name:    "malformed digest",
			item:    governance.ChangeSetItem{FieldPath: "a", Op: governance.ChangeAdd, AfterDigest: "md5:zz", AfterValue: json.RawMessage(`1`)},
			wantErr: true,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			err := testCase.item.Validate()
			if testCase.wantErr && err == nil {
				t.Fatalf("expected validation error")
			}
			if !testCase.wantErr && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}

func TestChangeSetEditabilityFollowsWorkflowState(t *testing.T) {
	if !governance.ChangeSetEditable(governance.ProposalDraft) {
		t.Fatal("draft change-sets must stay editable")
	}
	for _, state := range []governance.ProposalState{
		governance.ProposalProposed, governance.ProposalValidating,
		governance.ProposalInReview, governance.ProposalReleased, governance.ProposalRejected,
	} {
		if governance.ChangeSetEditable(state) {
			t.Fatalf("change-set must be frozen in state %s", state)
		}
	}
}
