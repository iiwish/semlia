//go:build unix

package local

import (
	"golang.org/x/sys/unix"
	"os"
)

func tryLockTemporary(file *os.File) bool {
	return unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB) == nil
}
