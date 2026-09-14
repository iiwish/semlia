// Package governance holds the M2 governed-authoring domain model: the SSOT
// §7.5 proposal state machine, structured change-sets, reviews, validation
// runs, recomputable policy decisions, immutable releases and the §8.6
// agent-behavior contract. Workflow state lives on proposals only; asset
// lifecycle, deployment and health states are different dimensions and are
// never collapsed into this state machine (semantic-asset design §4.8).
package governance

import "errors"

var (
	// ErrInvalidArgument rejects malformed commands and payloads before any
	// persistence happens.
	ErrInvalidArgument = errors.New("invalid governance argument")
	// ErrNotFound marks missing governance resources.
	ErrNotFound = errors.New("governance resource not found")
	// ErrConflict marks duplicate or racing writes (unique violations,
	// duplicate validator runs, repeated rollbacks, terminal mutations).
	ErrConflict = errors.New("governance state conflict")
	// ErrInvariant marks violations of persistence invariants: blocker
	// findings blocking release candidacy, immutable records, frozen
	// change-sets observed at the storage boundary.
	ErrInvariant = errors.New("governance invariant violated")
	// ErrInvalidTransition is the SSOT §7.5 state-machine error. It is
	// returned by the domain layer before any database write.
	ErrInvalidTransition = errors.New("invalid proposal state transition")
	// ErrChangeSetFrozen rejects change-set edits once a proposal left draft.
	ErrChangeSetFrozen = errors.New("proposal change-set is frozen once submitted")
	// ErrImmutableRecord refuses in-place mutation of submitted change-set
	// items, reviews, validation results, policy decisions, releases and
	// agent steps; the database triggers enforce the same invariant.
	ErrImmutableRecord = errors.New("governance record is immutable")

	// Production authoring errors (SP-T003 & SP-T004):
	ErrAlreadyProduced       = errors.New("production operation already produced")
	ErrIdentityConflict      = errors.New("production identity conflict")
	ErrIdempotencyConflict   = errors.New("idempotency key conflict")
	ErrInputIncomplete       = errors.New("production input incomplete")
	ErrContentMismatch       = errors.New("production content mismatch")
	ErrProductionSetRequired = errors.New("production set required")
	ErrLimitExceeded         = errors.New("production aggregate limit exceeded")
	ErrNoSubstantiveChange   = errors.New("no substantive change")
	ErrEvidenceMissing       = errors.New("evidence missing")
	ErrDependencyInvalid     = errors.New("dependency invalid")
)
