//go:build !darwin && !linux

package acceptance

import (
	"fmt"
	"runtime"
)

func createAcceptanceLock(string) (func() error, error) {
	return nil, fmt.Errorf("fresh-clone acceptance advisory lock is unsupported on %s", runtime.GOOS)
}
