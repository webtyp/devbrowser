package devbrowser_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"webtyp.com/devbrowser"
)

func TestProfileDir_StablePerProject(t *testing.T) {
	tmpCache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmpCache)

	root1 := "/mock/project/one"
	root2 := "/mock/project/two"

	dir1, err := devbrowser.ProfileDir(root1)
	if err != nil {
		t.Fatalf("devbrowser.ProfileDir(root1) failed: %v", err)
	}

	dir2, err := devbrowser.ProfileDir(root2)
	if err != nil {
		t.Fatalf("devbrowser.ProfileDir(root2) failed: %v", err)
	}

	if dir1 == dir2 {
		t.Errorf("expected different profile dirs for different roots, got same: %s", dir1)
	}

	absRelativeRoot, _ := filepath.Abs("./one")
	dir1Relative, _ := devbrowser.ProfileDir("./one")
	dir1RelativeAbs, _ := devbrowser.ProfileDir(absRelativeRoot)

	if dir1Relative != dir1RelativeAbs {
		t.Errorf("expected identical hashes for relative root and its absolute equivalent")
	}

	cacheDir, _ := os.UserCacheDir()
	expectedPrefix := filepath.Join(cacheDir, devbrowser.ProfilesDirName)
	if filepath.Dir(dir1) != expectedPrefix {
		t.Errorf("expected dir1 to be under %s, got %s", expectedPrefix, filepath.Dir(dir1))
	}
}

func TestProfileCleaner_IgnoresPersistentProfiles(t *testing.T) {
	tmpRoot := t.TempDir()

	// Create a regular throwaway profile
	throwawayDir := filepath.Join(tmpRoot, devbrowser.ProfilePrefix+"-test123")
	os.MkdirAll(throwawayDir, 0700)

	// Create a persistent profile
	persistentDir := filepath.Join(tmpRoot, "persistent-profile")
	os.MkdirAll(persistentDir, 0700)

	// Create dummy lock files to test stale grace if needed
	// Actually we want them to be stale so they get cleaned

	cleaner := devbrowser.NewProfileCleaner()
	cleaner.Root = tmpRoot
	cleaner.Grace = -1 // Immediate stale in our config if we bypass DefaultStaleGrace check, wait, Grace is defended!

	// Since Grace is defended against <= 0, we should set it to 1 microsecond and sleep,
	// OR override Now function.
	cleaner.Grace = 1 * time.Microsecond
	cleaner.Now = func() time.Time { return time.Now().Add(1 * time.Hour) }
	cleaner.ProcessAlive = func(pid int) bool { return false }

	removed, _, err := cleaner.CleanStale()
	if err != nil {
		t.Fatalf("CleanStale failed: %v", err)
	}

	if removed != 1 {
		t.Errorf("expected 1 removed directory, got %d", removed)
	}

	if _, err := os.Stat(throwawayDir); !os.IsNotExist(err) {
		t.Errorf("expected throwaway dir to be removed")
	}

	if _, err := os.Stat(persistentDir); err != nil {
		t.Errorf("expected persistent dir to remain, got err: %v", err)
	}
}
