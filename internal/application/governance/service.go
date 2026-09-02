// Package governance provides the M2 persistence-oriented application
// services over the governed-authoring domain: proposals (submit and decide),
// validation runs, recomputable policy decisions, immutable release cuts with
// rollback-as-new-release, and §8.6 agent-run recording. There are no HTTP
// concerns here; T003 layers the API contracts on top.
package governance

import (
	"time"

	"github.com/iiwish/semlia/pkg/identity"
)

type Clock interface{ Now() time.Time }

type ClockFunc func() time.Time

func (clock ClockFunc) Now() time.Time { return clock() }

// EventIDs bundles the two pre-minted event identities of one governance
// mutation so its audit fact and semlia.events/v1 outbox event commit inside
// one transaction (the M1 catalog discipline).
type EventIDs struct {
	AuditEventID  identity.EventID
	OutboxEventID identity.EventID
}

func newEventIDs() (EventIDs, error) {
	audit, err := identity.NewEventID()
	if err != nil {
		return EventIDs{}, err
	}
	outbox, err := identity.NewEventID()
	if err != nil {
		return EventIDs{}, err
	}
	return EventIDs{AuditEventID: audit, OutboxEventID: outbox}, nil
}
