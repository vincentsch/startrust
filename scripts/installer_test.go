package scripts

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var releaseAssets = []string{
	"startrust-linux-amd64", "startrust-linux-arm64",
	"startrust-darwin-amd64", "startrust-darwin-arm64",
	"startrust-windows-amd64.exe", "startrust-windows-arm64.exe",
}

// fakeRelease lays out release files the way GitHub serves them, so the
// installer can download them with a file:// base URL.
func fakeRelease(t *testing.T, corrupt bool) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "releases", "latest", "download")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var manifest strings.Builder
	for _, asset := range releaseAssets {
		body := "#!/bin/sh\necho 'startrust fixture " + asset + "'\n"
		if err := os.WriteFile(filepath.Join(dir, asset), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256([]byte(body))
		if corrupt {
			sum[0]++
		}
		fmt.Fprintf(&manifest, "%x  %s\n", sum, asset)
	}
	if err := os.WriteFile(filepath.Join(dir, "checksums.txt"), []byte(manifest.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func runInstaller(t *testing.T, root, installDir string) (string, error) {
	t.Helper()
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("curl not installed")
	}
	cmd := exec.Command("sh", "../install.sh")
	cmd.Env = append(os.Environ(), "STARTRUST_DOWNLOAD_BASE=file://"+root, "INSTALL_DIR="+installDir)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestInstallerVerifiesAndInstalls(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the installer needs a POSIX shell")
	}
	installDir := filepath.Join(t.TempDir(), "bin")
	out, err := runInstaller(t, fakeRelease(t, false), installDir)
	if err != nil {
		t.Fatalf("installer failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Installed the latest StarTrust release to "+filepath.Join(installDir, "startrust")) || !strings.Contains(out, "Try it: startrust check") {
		t.Errorf("installer output: %s", out)
	}
	got, err := exec.Command(filepath.Join(installDir, "startrust")).Output()
	if err != nil || !strings.Contains(string(got), "startrust-"+runtime.GOOS) {
		t.Errorf("installed the wrong file: %s %v", got, err)
	}
}

func TestInstallerSaysWhichDownloadFailed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the installer needs a POSIX shell")
	}
	// A release without files, as before the first release.
	out, err := runInstaller(t, t.TempDir(), filepath.Join(t.TempDir(), "bin"))
	if err == nil || !strings.Contains(out, "Could not download file://") || !strings.Contains(out, "that this release exists") {
		t.Fatalf("want a download failure that names the URL, got %v\n%s", err, out)
	}
}

func TestInstallerRejectsABadChecksum(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the installer needs a POSIX shell")
	}
	installDir := filepath.Join(t.TempDir(), "bin")
	out, err := runInstaller(t, fakeRelease(t, true), installDir)
	if err == nil || !strings.Contains(out, "Checksum mismatch") {
		t.Fatalf("want a checksum failure, got %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(installDir, "startrust")); err == nil {
		t.Error("a binary with a bad checksum was installed")
	}
}
