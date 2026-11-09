package github

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestFindTokenSaysWhereTheTokenCameFrom(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake GitHub CLI is a shell script")
	}
	// A fake GitHub CLI that has a login, like a real one that finds a
	// token in the system keyring.
	bin := t.TempDir()
	script := "#!/bin/sh\n[ \"$1 $2\" = \"auth token\" ] && echo gho_fromcli\n"
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	for _, tc := range []struct {
		env  map[string]string
		want Token
	}{
		{map[string]string{"GITHUB_TOKEN": "a", "GH_TOKEN": "b"}, Token{Value: "a", Source: "GITHUB_TOKEN"}},
		{map[string]string{"GITHUB_TOKEN": " ", "GH_TOKEN": "b"}, Token{Value: "b", Source: "GH_TOKEN"}},
		{nil, Token{Value: "gho_fromcli", Source: "gh"}},
	} {
		got := FindToken(context.Background(), func(name string) string { return tc.env[name] })
		if got != tc.want {
			t.Errorf("env %v: got %+v, want %+v", tc.env, got, tc.want)
		}
	}

	// Without the GitHub CLI there is no token at all.
	t.Setenv("PATH", t.TempDir())
	if got := FindToken(context.Background(), func(string) string { return "" }); got != (Token{}) {
		t.Errorf("got %+v without any token", got)
	}
}
