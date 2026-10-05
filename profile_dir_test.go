package devbrowser

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProfileDir_StablePerProject(t *testing.T) {
	tmpCache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmpCache)

	root1 := "/mock/project/one"
	root2 := "/mock/project/two"

	dir1, err := ProfileDir(root1)
	if err != nil {
		t.Fatalf("ProfileDir(root1) failed: %v", err)
	}

	dir2, err := ProfileDir(root2)
	if err != nil {
		t.Fatalf("ProfileDir(root2) failed: %v", err)
	}

	if dir1 == dir2 {
		t.Errorf("expected different profile dirs for different roots, got same: %s", dir1)
	}

	absRelativeRoot, _ := filepath.Abs("./one")
	dir1Relative, _ := ProfileDir("./one")
	dir1RelativeAbs, _ := ProfileDir(absRelativeRoot)

	if dir1Relative != dir1RelativeAbs {
		t.Errorf("expected identical hashes for relative root and its absolute equivalent")
	}

	cacheDir, _ := os.UserCacheDir()
	expectedPrefix := filepath.Join(cacheDir, ProfilesDirName)
	if filepath.Dir(dir1) != expectedPrefix {
		t.Errorf("expected dir1 to be under %s, got %s", expectedPrefix, filepath.Dir(dir1))
	}
}

func TestWithProfile_SetsUserDataDir(t *testing.T) {
	tmpCache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmpCache)

	tmpRoot := t.TempDir()

	b := &DevBrowser{}
	WithProfile(tmpRoot)(b)

	expectedDir, _ := ProfileDir(tmpRoot)

	if b.ProfileDir != expectedDir {
		t.Errorf("expected ProfileDir to be %s, got %s", expectedDir, b.ProfileDir)
	}

	// Verify directory creation mode
	info, err := os.Stat(expectedDir)
	if err != nil {
		t.Fatalf("expected directory to be created, stat failed: %v", err)
	}
	if info.Mode().Perm() != 0700 {
		t.Errorf("expected directory mode 0700, got %v", info.Mode().Perm())
	}

	flags := parseExecAllocatorOptions(b.buildAllocatorOptions())
	val, exists := flags["user-data-dir"]
	if !exists || val != expectedDir {
		t.Errorf("expected flag user-data-dir=%s, got %v (exists=%v)", expectedDir, val, exists)
	}
}

func TestWithProfile_LockedFallsBack(t *testing.T) {
	tmpCache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmpCache)

	tmpRoot := t.TempDir()
	expectedDir, _ := ProfileDir(tmpRoot)
	os.MkdirAll(expectedDir, 0700)

	// Create a fake lock pointing to this process
	lockPath := filepath.Join(expectedDir, singletonLockName)
	target := fmt.Sprintf("hostname-%d", os.Getpid())
	err := os.Symlink(target, lockPath)
	if err != nil {
		t.Fatalf("symlink failed: %v", err)
	}

	b := &DevBrowser{}
	WithProfile(tmpRoot)(b)

	if b.ProfileDir != expectedDir {
		t.Errorf("expected ProfileDir to be set, got %s", b.ProfileDir)
	}

	flags := parseExecAllocatorOptions(b.buildAllocatorOptions())
	_, exists := flags["user-data-dir"]
	if exists {
		t.Errorf("expected no user-data-dir flag because profile is locked")
	}

	if b.profileFallbackReason == "" {
		t.Errorf("expected profileFallbackReason to be set")
	}
}

func TestBuildAllocatorOptions_NoProfileByDefault(t *testing.T) {
	b := &DevBrowser{}
	flags := parseExecAllocatorOptions(b.buildAllocatorOptions())
	if val, exists := flags["user-data-dir"]; exists {
		t.Errorf("expected no user-data-dir by default, got %v", val)
	}
}

func TestProfileCleaner_IgnoresPersistentProfiles(t *testing.T) {
	tmpRoot := t.TempDir()

	// Create a regular throwaway profile
	throwawayDir := filepath.Join(tmpRoot, ProfilePrefix+"-test123")
	os.MkdirAll(throwawayDir, 0700)

	// Create a persistent profile
	persistentDir := filepath.Join(tmpRoot, "persistent-profile")
	os.MkdirAll(persistentDir, 0700)

	// Create dummy lock files to test stale grace if needed
	// Actually we want them to be stale so they get cleaned

	cleaner := NewProfileCleaner()
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
