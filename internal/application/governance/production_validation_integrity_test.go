package governance

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

func TestProductionValidationSealBindsAttemptAndRejectsHiddenBlocker(t *testing.T) {
	w, _ := identity.NewWorkspaceID()
	op, _ := identity.NewProductionOperationID()
	p, _ := identity.NewProposalID()
	digest := "sha256:" + strings.Repeat("1", 64)
	completion := ProductionValidationCompletion{Attempt: domain.ValidationAttempt{WorkspaceID: w, OperationID: op, ProductionVersion: 1, AttemptNo: 1, SetDigest: digest, InputDigest: digest, RequiredChecksDigest: digest, FreshnessDigest: digest}, Checks: []ProductionCheckResult{{Check: ProductionCheck{p.String(), "schema", ProductionValidatorVersion}, InputDigest: digest, Status: domain.ValidationSucceeded, Findings: []Finding{{Severity: domain.SeverityInfo, Code: "SCHEMA_CHECK_COMPLETED", Message: "complete", InputDigest: digest, Details: json.RawMessage(`{}`)}}}}}
	one, err := SealProductionValidation(completion, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	completion.Attempt.AttemptNo = 2
	two, err := SealProductionValidation(completion, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if *one.Attempt.ValidationDigest == *two.Attempt.ValidationDigest {
		t.Fatal("different semantic attempts share validation digest")
	}
	completion.Checks[0].Findings[0].Severity = domain.SeverityBlocker
	if _, err := SealProductionValidation(completion, time.Now()); err == nil {
		t.Fatal("succeeded seal hid blocker finding")
	}
}
