//go:build !darwin && !linux

package acceptance

import (
	"context"
	"os/exec"
)

// Semlia's fresh-clone acceptance target supports macOS and Linux. Other
// platforms retain direct-child cancellation so the package remains buildable.
func newAcceptanceCommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	command := exec.CommandContext(ctx, name, args...)
	command.WaitDelay = acceptanceCommandWaitDelay
	return command
}
