package devbrowser

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

// markProfileExitedCleanly rewrites <dir>/Default/Preferences so Chrome
// believes its last run closed normally. Without it, a daemon killed without
// closing the browser leaves exit_type "Crashed" and every start shows
// "Restore pages?". A missing file is fine (fresh profile); an unreadable or
// invalid one is left untouched and reported — the profile holds the
// developer's logins and is never deleted or rewritten blindly.
func markProfileExitedCleanly(dir string) error {
	prefsPath := filepath.Join(dir, "Default", "Preferences")
	data, err := os.ReadFile(prefsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var prefs map[string]any
	if err := json.Unmarshal(data, &prefs); err != nil {
		return err
	}

	profileObj, ok := prefs["profile"].(map[string]any)
	if !ok {
		profileObj = make(map[string]any)
		prefs["profile"] = profileObj
	}

	profileObj["exit_type"] = "Normal"
	profileObj["exited_cleanly"] = true

	out, err := json.Marshal(prefs)
	if err != nil {
		return err
	}

	tmpPath := prefsPath + ".tmp"
	if err := os.WriteFile(tmpPath, out, 0600); err != nil {
		return err
	}

	return os.Rename(tmpPath, prefsPath)
}
