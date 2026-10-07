// Root-level test (justified): exercises buildAllocatorOptions and singletonLockName — the profile directory and lock handling are decided before launch and not observable through the public API.
package devbrowser

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

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
