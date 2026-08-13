//go:build darwin || linux

package acceptance

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

func acceptanceJourneySignalContext(parent context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(parent, os.Interrupt, syscall.SIGHUP, syscall.SIGTERM)
}
