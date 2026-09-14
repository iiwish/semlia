package identity

import (
	auth "github.com/iiwish/semlia/internal/domain/authorization"
	"strings"
	"testing"
)

func TestMachineTokenEntropyAndVerifier(t *testing.T) {
	first, err := newMachineToken("ccd_test")
	if err != nil {
		t.Fatal(err)
	}
	second, err := newMachineToken("ccd_test")
	if err != nil {
		t.Fatal(err)
	}
	if first == second || len(strings.Split(first, ".")[1]) < 43 {
		t.Fatal("token lacks independent 256-bit entropy")
	}
	digest := machineDigest(first)
	if !verifyMachineToken(first, digest) || verifyMachineToken(second, digest) {
		t.Fatal("verifier does not distinguish tokens")
	}
	if strings.Contains(string(digest[:]), first) {
		t.Fatal("stored plaintext")
	}
}

func TestCredentialActionSetIsBoundedDistinctAndKnown(t *testing.T) {
	for _, test := range []struct {
		name    string
		actions []auth.Action
		want    bool
	}{
		{"empty", nil, false}, {"read", []auth.Action{auth.ActionAssetRead}, true},
		{"resolve", []auth.Action{auth.ActionSemanticResolve}, true}, {"both", []auth.Action{auth.ActionSemanticResolve, auth.ActionAssetRead}, true},
		{"duplicate", []auth.Action{auth.ActionAssetRead, auth.ActionAssetRead}, false},
		{"over-contract-limit", []auth.Action{auth.ActionAssetRead, auth.ActionSemanticResolve, auth.ActionAssetRead}, false},
		{"execute-explicit-opt-in", []auth.Action{auth.ActionSemanticExecute}, true},
		{"all-three", []auth.Action{auth.ActionSemanticExecute, auth.ActionSemanticResolve, auth.ActionAssetRead}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if validCredentialActions(test.actions) != test.want {
				t.Fatal("invalid credential action set accepted or valid set rejected")
			}
		})
	}
}
