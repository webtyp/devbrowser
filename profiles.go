package devbrowser

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ProfilePrefix is the name prefix the vendored allocator gives every throwaway
// Chrome profile directory it creates.
const ProfilePrefix = "chromedp-runner"

// singletonLockName is the symlink Chrome maintains in a profile root while it
// is running. Its target is "<hostname>-<pid>".
const singletonLockName = "SingletonLock"

// DefaultStaleGrace is how long a profile directory with no lock is left alone,
// so a browser that is still starting up is never swept from under itself.
const DefaultStaleGrace = 5 * time.Minute

// ProfileCleaner removes Chrome profile directories left behind by browsers
// whose owning process is gone. Zero value is not usable; build it with
// NewProfileCleaner and override the fields in tests.
type ProfileCleaner struct {
	Root         string             // where profiles live; default os.TempDir()
	Grace        time.Duration      // default DefaultStaleGrace
	Now          func() time.Time   // default time.Now
	ProcessAlive func(pid int) bool // default processAlive
}

// NewProfileCleaner initializes a new ProfileCleaner with sensible defaults.
func NewProfileCleaner() *ProfileCleaner {
	return &ProfileCleaner{
		Root:         os.TempDir(),
		Grace:        DefaultStaleGrace,
		Now:          time.Now,
		ProcessAlive: processAlive,
	}
}

// dirSize calculates total bytes of files inside dir. Errors contribute 0.
func dirSize(dir string) int64 {
	var size int64
	_ = filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size
}

// CleanStale removes every stale profile directory under Root and reports how
// many went and how many bytes that freed. A directory is stale when it has no
// SingletonLock and is older than Grace, or when the pid in its SingletonLock
// is not a live process. Errors on individual directories are skipped, not
// returned: a sweep must never fail a caller's startup.
func (c *ProfileCleaner) CleanStale() (removed int, freed int64, err error) {
	root := c.Root
	if root == "" {
		root = os.TempDir()
	}

	entries, readErr := os.ReadDir(root)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			return 0, 0, nil
		}
		return 0, 0, readErr
	}

	now := c.Now
	if now == nil {
		now = time.Now
	}

	isAlive := c.ProcessAlive
	if isAlive == nil {
		isAlive = processAlive
	}

	// Grace is defended like every other field: a zero value here would make
	// each lock-less directory instantly stale and sweep a browser that is
	// still starting up, which is the one thing the grace window exists for.
	grace := c.Grace
	if grace <= 0 {
		grace = DefaultStaleGrace
	}

	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), ProfilePrefix) {
			continue
		}

		dirPath := filepath.Join(root, entry.Name())
		lockPath := filepath.Join(dirPath, singletonLockName)

		pid, hasLock, readable := lockOwner(lockPath)
		stale := false

		if hasLock {
			if !readable {
				continue // cannot tell -> treat the directory as in use
			}
			stale = !isAlive(pid)
		} else {
			// Readlink failed (no lock or error). Check modtime against Grace.
			info, statErr := entry.Info()
			if statErr != nil {
				continue
			}
			if now().Sub(info.ModTime()) > grace {
				stale = true
			}
		}

		if stale {
			sz := dirSize(dirPath)
			if removeErr := os.RemoveAll(dirPath); removeErr == nil {
				removed++
				freed += sz
			}
		}
	}

	return removed, freed, nil
}

// lockOwner reads a Chrome SingletonLock symlink, whose target is "<hostname>-<pid>". hasLock is
// false when there is no lock; readable is false when its target cannot be parsed.
func lockOwner(lockPath string) (pid int, hasLock, readable bool) {
	target, err := os.Readlink(lockPath)
	if err != nil {
		return 0, false, false
	}
	idx := strings.LastIndex(target, "-")
	if idx == -1 || idx == len(target)-1 {
		return 0, true, false
	}
	pid, err = strconv.Atoi(target[idx+1:])
	if err != nil {
		return 0, true, false
	}
	return pid, true, true
}
