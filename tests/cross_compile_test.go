package devbrowser_test

import (
	"os"
	"os/exec"
	"testing"
)

// devbrowser ships inside the webtyp binary, which is released for every OS
// below. A per-OS file (process_windows.go, process_unix.go) is only compiled
// for its own OS, so a local test run never sees an error in the others —
// this test does.
func TestCompilesForEveryReleaseOS(t *testing.T) {
	gobin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not on PATH")
	}
	for _, target := range []struct{ goos, goarch string }{
		{"linux", "amd64"},
		{"darwin", "arm64"},
		{"windows", "amd64"},
	} {
		t.Run(target.goos, func(t *testing.T) {
			cmd := exec.Command(gobin, "build", "..")
			cmd.Env = append(os.Environ(), "GOOS="+target.goos, "GOARCH="+target.goarch, "CGO_ENABLED=0")
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("GOOS=%s GOARCH=%s: %v\n%s", target.goos, target.goarch, err, out)
			}
		})
	}
}
