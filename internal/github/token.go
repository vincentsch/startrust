package github

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// Token is a GitHub token and where it came from.
type Token struct {
	Value  string
	Source string // "GITHUB_TOKEN", "GH_TOKEN", "gh", or "" for none
}

// FindToken looks for a token in GITHUB_TOKEN, then GH_TOKEN, then asks the
// GitHub CLI (`gh auth token`). It returns an empty Token when there is none;
// StarTrust then works anonymously.
func FindToken(ctx context.Context, getenv func(string) string) Token {
	for _, name := range []string{"GITHUB_TOKEN", "GH_TOKEN"} {
		if value := strings.TrimSpace(getenv(name)); value != "" {
			return Token{Value: value, Source: name}
		}
	}
	if _, err := exec.LookPath("gh"); err != nil {
		return Token{}
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "gh", "auth", "token").Output()
	value := strings.TrimSpace(string(out))
	if err != nil || value == "" || strings.ContainsAny(value, " \t\r\n") {
		return Token{}
	}
	return Token{Value: value, Source: "gh"}
}
