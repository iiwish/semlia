package governance

import (
	"encoding/json"
	"time"

	"github.com/iiwish/semlia/pkg/identity"
)

// ValidationSeverity is the AssetTypeProfile severity enum
// (semantic-asset design §3.3). Blocker findings block release candidacy.
type ValidationSeverity string

const (
	SeverityBlocker       ValidationSeverity = "blocker"
	SeverityWarning       ValidationSeverity = "warning"
	SeverityInfo          ValidationSeverity = "info"
	SeverityNotApplicable ValidationSeverity = "not_applicable"
)

func (severity ValidationSeverity) Valid() bool {
	switch severity {
	case SeverityBlocker, SeverityWarning, SeverityInfo, SeverityNotApplicable:
		return true
	}
	return false
}

// BlocksRelease reports the §3.3 publication behavior: only a Blocker finding
// prevents a candidate from being released.
func (severity ValidationSeverity) BlocksRelease() bool {
	return severity == SeverityBlocker
}

type ValidationStatus string

const (
	ValidationRunning   ValidationStatus = "running"
	ValidationSucceeded ValidationStatus = "succeeded"
	ValidationFailed    ValidationStatus = "failed"
	ValidationCancelled ValidationStatus = "cancelled"
)

func (status ValidationStatus) Terminal() bool {
	return status == ValidationSucceeded || status == ValidationFailed || status == ValidationCancelled
}

// ValidationRun is one deterministic validator execution for one proposal.
// The data-model invariant "one run per proposal per validator version" is
// enforced by a unique index; runs never mutate revisions.
type ValidationRun struct {
	ID               identity.ValidationRunID
	WorkspaceID      identity.WorkspaceID
	ProposalID       identity.ProposalID
	ValidatorID      string
	ValidatorVersion string
	Status           ValidationStatus
	StartedAt        time.Time
	FinishedAt       *time.Time
}

type ValidationResult struct {
	ID              identity.ValidationResultID
	WorkspaceID     identity.WorkspaceID
	ValidationRunID identity.ValidationRunID
	Severity        ValidationSeverity
	Code            string
	Message         string
	InputDigest     string
	Details         json.RawMessage
	CreatedAt       time.Time
}

func (result ValidationResult) Validate() error {
	if result.ID.IsZero() || result.WorkspaceID.IsZero() || result.ValidationRunID.IsZero() ||
		!result.Severity.Valid() || result.Code == "" || result.Message == "" || !isContentDigest(result.InputDigest) {
		return ErrInvalidArgument
	}
	return nil
}
