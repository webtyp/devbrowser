package devbrowser

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// ProfilesDirName is where persistent profiles live, under the user's cache directory.
const ProfilesDirName = "webtyp/devbrowser/profiles"

// ProfileDir returns the persistent profile directory of the project at root:
// <os.UserCacheDir()>/webtyp/devbrowser/profiles/<first 12 hex chars of SHA-256 of the absolute root>.
// It does not create it.
func ProfileDir(root string) (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("user cache dir: %w", err)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("absolute root: %w", err)
	}

	hash := sha256.Sum256([]byte(absRoot))
	hashStr := hex.EncodeToString(hash[:])[:12]

	return filepath.Join(cacheDir, ProfilesDirName, hashStr), nil
}

// WithProfile makes the browser use ProfileDir(root) as its Chrome profile (created with 0700 when
// missing), so storage (OPFS, IndexedDB, cookies) survives between sessions. When the directory
// cannot be resolved or created, or another live Chrome holds it (its SingletonLock points to a
// running pid), the browser falls back to a throwaway profile and logs why.
func WithProfile(root string) Option {
	return func(b *DevBrowser) {
		dir, err := ProfileDir(root)
		if err != nil {
			b.profileFallbackReason = err.Error()
			return
		}

		if err := os.MkdirAll(dir, 0700); err != nil {
			b.profileFallbackReason = fmt.Sprintf("mkdir: %v", err)
			return
		}

		b.ProfileDir = dir
	}
}
