//go:build !darwin && !linux

package acceptance

import "context"

func acceptanceJourneySignalContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithCancel(parent)
}
