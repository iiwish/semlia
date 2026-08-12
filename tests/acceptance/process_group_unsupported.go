//go:build !darwin && !linux

package acceptance

import (
	"context"
	"os/exec"
	"time"
)

// Semlia's fresh-clone acceptance target supports macOS and Linux. Other
// platforms retain direct-child cancellation so the package remains buildable.
func acceptanceCommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	command := exec.CommandContext(ctx, name, args...)
	command.WaitDelay = 2 * time.Second
	return command
}
