package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestActionWritesOutputsAndBadge runs the Action's script the way GitHub
// does, in a fresh checkout without a badge folder. The check is faked (it
// writes a saved report and exits 7, as --fail-on does); report and badge
// are the real binary.
func TestActionWritesOutputsAndBadge(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the action script needs bash")
	}
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not installed")
	}
	tmp := t.TempDir()
	real := filepath.Join(tmp, "startrust-real")
	if out, err := exec.Command("go", "build", "-o", real, "../cmd/startrust").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	saved, err := filepath.Abs("../internal/cli/testdata/check-saved.json")
	if err != nil {
		t.Fatal(err)
	}
	fake := filepath.Join(tmp, "fake-startrust")
	script := `#!/usr/bin/env bash
if [ "$1" = check ]; then
    while [ $# -gt 0 ]; do
        if [ "$1" = --out ]; then cp "` + saved + `" "$2"; fi
        shift
    done
    exit 7
fi
exec "` + real + `" "$@"
`
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	checkout := filepath.Join(tmp, "checkout")
	if err := os.MkdirAll(checkout, 0o755); err != nil {
		t.Fatal(err)
	}
	outputs, summary := filepath.Join(tmp, "outputs"), filepath.Join(tmp, "summary.md")
	action, _ := filepath.Abs("action.sh")
	cmd := exec.Command("bash", action)
	cmd.Dir = checkout
	cmd.Env = append(os.Environ(),
		"STARTRUST_BIN="+fake, "RUNNER_TEMP="+tmp,
		"INPUT_REPOSITORY=vincentsch/explainroo", "INPUT_FAIL_ON=high", "INPUT_BADGE=badge/stars.json", "INPUT_ARGS=",
		"GITHUB_OUTPUT="+outputs, "GITHUB_STEP_SUMMARY="+summary)
	out, err := cmd.CombinedOutput()
	if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 7 {
		t.Fatalf("want exit 7 from --fail-on, got %v\n%s", err, out)
	}
	written, _ := os.ReadFile(outputs)
	for _, want := range []string{"risk=unknown", "verdict=Too new to judge.", "report=" + tmp} {
		if !strings.Contains(string(written), want) {
			t.Errorf("step outputs are missing %q:\n%s", want, written)
		}
	}
	if markdown, _ := os.ReadFile(summary); !strings.Contains(string(markdown), "## Stars of") {
		t.Errorf("job summary:\n%s", markdown)
	}
	if badge, err := os.ReadFile(filepath.Join(checkout, "badge", "stars.json")); err != nil || !strings.Contains(string(badge), `"message": "too new to judge"`) {
		t.Errorf("badge: %s %v", badge, err)
	}
	if !strings.Contains(string(out), "::warning title=StarTrust could not judge vincentsch/explainroo::") {
		t.Errorf("an unknown risk should raise a warning:\n%s", out)
	}
}
