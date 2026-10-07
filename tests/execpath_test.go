package devbrowser_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"webtyp.com/devbrowser"
)

func TestResolveChromeExecPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows due to shell script incompatibility")
	}

	// Create a temporary directory for our fake binaries
	tmpDir, err := os.MkdirTemp("", "execpath-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a crashing 'chromium' script
	chromiumPath := filepath.Join(tmpDir, "chromium")
	err = os.WriteFile(chromiumPath, []byte("#!/bin/sh\nexit 1\n"), 0755)
	if err != nil {
		t.Fatalf("Failed to create chromium script: %v", err)
	}

	// Create a working 'google-chrome' script
	chromePath := filepath.Join(tmpDir, "google-chrome")
	err = os.WriteFile(chromePath, []byte("#!/bin/sh\necho \"Google Chrome 120.0.6099.109\"\nexit 0\n"), 0755)
	if err != nil {
		t.Fatalf("Failed to create google-chrome script: %v", err)
	}

	// Save original PATH and restore it after test
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)

	// Set PATH to our temp dir
	os.Setenv("PATH", tmpDir)

	// Also clear CHROME_EXECPATH if set
	oldExecPath := os.Getenv("CHROME_EXECPATH")
	defer os.Setenv("CHROME_EXECPATH", oldExecPath)
	os.Unsetenv("CHROME_EXECPATH")

	// Execute resolution
	resolved := devbrowser.ResolveChromeExecPath()

	// It should have resolved to our working 'google-chrome'
	if resolved != chromePath {
		t.Errorf("Expected to resolve to %s, but got %s", chromePath, resolved)
	}

	// Now test CHROME_EXECPATH override
	os.Setenv("CHROME_EXECPATH", chromePath)
	resolved = devbrowser.ResolveChromeExecPath()
	if resolved != chromePath {
		t.Errorf("Expected to honor CHROME_EXECPATH %s, but got %s", chromePath, resolved)
	}

	// Test CHROME_EXECPATH pointing to a broken one
	os.Setenv("CHROME_EXECPATH", chromiumPath)
	resolved = devbrowser.ResolveChromeExecPath()
	// Since CHROME_EXECPATH is broken, it should fall back to searching PATH and find google-chrome
	if resolved != chromePath {
		t.Errorf("Expected to skip broken CHROME_EXECPATH and resolve to %s, but got %s", chromePath, resolved)
	}
}

func TestResolveChromeExecPath_Order(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows due to shell script incompatibility")
	}

	tmpDir, err := os.MkdirTemp("", "execpath-test-order")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Both are working scripts
	chromiumPath := filepath.Join(tmpDir, "chromium")
	os.WriteFile(chromiumPath, []byte("#!/bin/sh\necho \"Chromium 120.0.6099.109\"\nexit 0\n"), 0755)

	chromePath := filepath.Join(tmpDir, "google-chrome")
	os.WriteFile(chromePath, []byte("#!/bin/sh\necho \"Google Chrome 120.0.6099.109\"\nexit 0\n"), 0755)

	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)
	os.Setenv("PATH", tmpDir)

	oldExecPath := os.Getenv("CHROME_EXECPATH")
	defer os.Setenv("CHROME_EXECPATH", oldExecPath)
	os.Unsetenv("CHROME_EXECPATH")

	// Even if both work, google-chrome should be preferred
	resolved := devbrowser.ResolveChromeExecPath()
	if resolved != chromePath {
		t.Errorf("Expected to prefer google-chrome over chromium, but got %s", resolved)
	}
}
