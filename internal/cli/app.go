// Package cli defines the startrust commands.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	rungrad "github.com/vincentsch/rungrad"
	"golang.org/x/term"

	"github.com/vincentsch/startrust/internal/archive"
	"github.com/vincentsch/startrust/internal/cache"
	"github.com/vincentsch/startrust/internal/check"
	"github.com/vincentsch/startrust/internal/explain"
	"github.com/vincentsch/startrust/internal/github"
	"github.com/vincentsch/startrust/internal/render"
	"github.com/vincentsch/startrust/internal/version"
)

// ExitPolicy is the exit code when --fail-on matches.
const ExitPolicy = 7

// Services lets tests point the commands at fake servers.
type Services struct {
	GitHubURL  string
	ArchiveURL string
	HNURL      string
	Getenv     func(string) string
	Now        func() time.Time
	// FindToken looks up a GitHub token; nil uses the environment and the
	// GitHub CLI.
	FindToken func(context.Context) github.Token
	// Terminal forces progress output on or off; nil detects it.
	Terminal *bool
}

// Run runs the CLI with a context that is canceled on Ctrl-C.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	app := NewApp(Services{})
	app.Root().SetContext(ctx)
	return app.Run(args, stdout, stderr)
}

// NewApp builds the command tree.
func NewApp(services Services) *rungrad.App {
	if services.Getenv == nil {
		services.Getenv = os.Getenv
	}
	if services.Now == nil {
		services.Now = time.Now
	}
	app := rungrad.New(rungrad.AppConfig{
		Name:  "startrust",
		Short: "Check if a GitHub repo's stars look bought",
		Long: "StarTrust checks whether a GitHub repo's stars look bought. It reads GitHub's daily star\n" +
			"counts and marks unusual spikes. Then it checks the accounts behind the busiest days in\n" +
			"GH Archive, a public record of GitHub events. Any public repo works, with or without a\n" +
			"GitHub token.",
		Version: version.String(),
	})
	app.Root().Example = "startrust check junegunn/fzf\nstartrust check astral-sh/uv --details\nstartrust compare charmbracelet/glow junegunn/fzf"
	// Every rungrad tool accepts --config and --dry-run. StarTrust has no
	// settings to put in a file, and only cache prune deletes anything, so
	// the help says what the two flags do here.
	if flag := app.Root().PersistentFlags().Lookup("config"); flag != nil {
		flag.Usage = "Ignored, because StarTrust has no config file"
	}
	if flag := app.Root().PersistentFlags().Lookup("dry-run"); flag != nil {
		flag.Usage = "Show what cache prune would delete without deleting it (the other commands ignore it)"
	}
	app.AddCommand(
		checkCommand(services),
		compareCommand(services),
		reportCommand(services),
		badgeCommand(services),
		cacheCommand(services),
	)
	return app
}

// apiError gives GitHub and file errors the right exit code. A
// --former-name that GitHub does not forward to the repo is a usage error.
func apiError(err error) error {
	if errors.As(err, new(check.FormerNameError)) {
		return rungrad.NewError(rungrad.ExitUsage, err.Error())
	}
	var apiErr *github.Error
	if errors.As(err, &apiErr) {
		code := rungrad.ExitAPI
		switch apiErr.Kind {
		case github.ErrNotFound:
			code = rungrad.ExitNotFound
		case github.ErrAuth:
			code = rungrad.ExitAuth
		case github.ErrForbidden:
			code = rungrad.ExitForbidden
		case github.ErrRateLimited, github.ErrBudget:
			code = rungrad.ExitRateLimited
		case github.ErrCanceled:
			code = ExitInterrupted
		}
		return rungrad.NewError(code, err.Error())
	}
	return rungrad.NewError(rungrad.ExitAPI, err.Error())
}

func usageError(format string, args ...any) error {
	return rungrad.NewError(rungrad.ExitUsage, fmt.Sprintf(format, args...))
}

// runner builds a check runner from the command's flags.
func runner(f *rungrad.Factory, cmd *cobra.Command, services Services) (*check.Runner, error) {
	maxRequests, _ := cmd.Flags().GetInt("max-requests")
	if maxRequests < 1 {
		return nil, usageError("--max-requests must be at least 1")
	}
	noToken, _ := cmd.Flags().GetBool("no-token")
	deep, _ := cmd.Flags().GetBool("deep")
	if noToken && deep {
		return nil, usageError("--deep needs a token with push access to the repo, so it cannot run with --no-token")
	}
	// With --no-token, StarTrust does not even ask the GitHub CLI.
	var token github.Token
	switch {
	case noToken:
	case services.FindToken != nil:
		token = services.FindToken(cmd.Context())
	default:
		token = github.FindToken(cmd.Context(), services.Getenv)
	}
	f.RegisterSecret(token.Value)
	run := &check.Runner{
		GitHub:      &github.Client{BaseURL: services.GitHubURL, Token: token.Value, MaxRequests: maxRequests, UserAgent: "startrust/" + version.Version},
		TokenSource: token.Source,
		Deep:        deep,
		Now:         services.Now,
		Progress:    newProgress(f, services),
	}
	if noArchive, _ := cmd.Flags().GetBool("no-archive"); !noArchive {
		run.Archive = &archive.Client{Endpoint: services.ArchiveURL}
	}
	if explainFlag, _ := cmd.Flags().GetBool("explain"); explainFlag {
		run.HN = &explain.HN{Endpoint: services.HNURL}
	}
	store, err := openCache(cmd)
	if err != nil {
		return nil, err
	}
	run.Cache = store
	return run, nil
}

func addRunFlags(cmd *cobra.Command) {
	cmd.Flags().Bool("no-archive", false, "Leave out the account check in GH Archive (the verdict then rests on the timing of the stars)")
	cmd.Flags().Bool("explain", false, "Look for Hacker News posts and releases near the busiest days")
	cmd.Flags().Int("max-requests", 1000, "Stop after this many GitHub requests and report what was read")
	cmd.Flags().Bool("no-token", false, "Check without a GitHub token, even when one is set or you are logged in with the GitHub CLI")
	addCacheFlag(cmd)
}

func addCacheFlag(cmd *cobra.Command) {
	cmd.Flags().String("cache-dir", "", "Folder for saved checks (default: startrust in your user cache directory)")
}

func openCache(cmd *cobra.Command) (*cache.Store, error) {
	root, _ := cmd.Flags().GetString("cache-dir")
	if root == "" {
		var err error
		if root, err = cache.DefaultRoot(); err != nil {
			return nil, rungrad.NewError(rungrad.ExitAPI, err.Error())
		}
	}
	store, err := cache.Open(root)
	if err != nil {
		return nil, rungrad.NewError(rungrad.ExitAPI, err.Error())
	}
	return store, nil
}

// parseRepo accepts owner/name or a GitHub URL.
func parseRepo(arg string) (string, error) {
	name, ok := github.ParseName(arg)
	if !ok {
		return "", usageError("%q is not a GitHub repo; use owner/name, for example junegunn/fzf", arg)
	}
	return name, nil
}

// progress prints status lines on stderr while a terminal user waits. It is
// silent for --json, --quiet, and when output goes to a file or pipe.
type progress struct {
	out     io.Writer
	enabled bool
	width   int
}

func newProgress(f *rungrad.Factory, services Services) *progress {
	enabled := !f.Flags.JSON && !f.Flags.Quiet
	if services.Terminal != nil {
		enabled = enabled && *services.Terminal
	} else {
		enabled = enabled && isTerminal(f.Stdout) && isTerminal(f.Stderr)
	}
	return &progress{out: f.Stderr, enabled: enabled}
}

func isTerminal(w io.Writer) bool {
	file, ok := w.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}

// Status overwrites the current line. A carriage return works in every
// terminal, including the Windows console.
func (p *progress) Status(message string) {
	if !p.enabled {
		return
	}
	if len(message) > 78 {
		message = message[:77] + "…"
	}
	padding := max(0, p.width-len([]rune(message)))
	fmt.Fprintf(p.out, "\r%s%s\r%s", message, strings.Repeat(" ", padding), message)
	p.width = len([]rune(message))
	if message == "" {
		p.width = 0
	}
}

// Note prints a line that stays.
func (p *progress) Note(message string) {
	if !p.enabled {
		return
	}
	p.Status("")
	fmt.Fprintln(p.out, render.Wrap(message, 80, ""))
}
