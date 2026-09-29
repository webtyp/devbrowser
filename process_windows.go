//go:build windows

package devbrowser

import (
	"os"
)

// processAlive reports whether pid names a running process. On Windows
// os.FindProcess opens a handle to the process and fails if it does not exist,
// so success means alive; the handle is released right away.
func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	_ = p.Release()
	return true
}
