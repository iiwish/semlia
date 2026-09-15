package local

import (
	"golang.org/x/sys/windows"
	"os"
)

func tryLockTemporary(file *os.File) bool {
	return windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &windows.Overlapped{}) == nil
}
