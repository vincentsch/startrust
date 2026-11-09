package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	rungrad "github.com/vincentsch/rungrad"

	"github.com/vincentsch/startrust/internal/cache"
	"github.com/vincentsch/startrust/internal/check"
	"github.com/vincentsch/startrust/internal/github"
	"github.com/vincentsch/startrust/internal/render"
)

func checkCommand(services Services) *rungrad.Command {
	return &rungrad.Command{
		Use:   "check <owner/repo>",
		Short: "Check whether a repo's stars look bought",
		Long: "Check reads the repo's daily star counts from GitHub and draws them as a chart. It\n" +
			"marks unusual spikes and looks up the accounts behind the busiest days in GH Archive, a\n" +
			"public record of GitHub events. It prints the verdict first, then the evidence.\n\n" +
			"Below the verdict you see the repo's activity, such as its forks and recent issues. They\n" +
			"help when a repo is too new for StarTrust to judge, but they never change the risk.\n\n" +
			"You need no setup. Without a token, GitHub allows 60 requests an hour, which is enough\n" +
			"for one or two checks. If GITHUB_TOKEN or GH_TOKEN is set, or you are logged in with the\n" +
			"GitHub CLI, StarTrust uses that token. The last lines of the report say which one it\n" +
			"used and how many requests were left. Add --no-token to check without one.\n\n" +
			"GH Archive files stars under the name a repo had at the time. If the repo was renamed,\n" +
			"add its old name with --former-name, and one check covers its whole history. Checking\n" +
			"the old name itself works too, because GitHub forwards it to the repo.\n\n" +
			"Maintainers can add --deep to compare the profiles of the people who starred their repo.",
		Args: oneArg("check takes one repo, such as junegunn/fzf; use `startrust compare` for several"),
		Examples: []string{
			"startrust check junegunn/fzf",
			"startrust check https://github.com/charmbracelet/glow --details",
			"startrust check owner/repo --markdown --out stars.md",
			"startrust check owner/repo --json",
			"startrust check owner/repo --fail-on high",
			"startrust check owner/repo --no-token",
			"startrust check withastro/astro --former-name snowpackjs/astro",
		},
		OutputModes: []string{rungrad.OutputModeHuman, rungrad.OutputModeJSON},
		Related:     []string{"startrust compare", "startrust report", "startrust badge"},
		Configure: func(cmd *cobra.Command) {
			cmd.Flags().Bool("details", false, "Show the full evidence (and include it in --json)")
			cmd.Flags().Bool("markdown", false, "Write a Markdown report for sharing")
			cmd.Flags().String("out", "", "Write the report to this file instead of the terminal")
			cmd.Flags().Bool("deep", false, "Also list who starred and compare their profiles (needs push access to the repo)")
			cmd.Flags().String("fail-on", "", "Exit with code 7 when the risk is at least this level: low, medium or high (an unknown risk never matches)")
			cmd.Flags().StringArray("former-name", nil, "An earlier `owner/name` of the repo, also looked up in GH Archive (repeat it for several)")
			addRunFlags(cmd)
		},
		Run: func(f *rungrad.Factory, cmd *cobra.Command, args []string) error {
			name, err := parseRepo(args[0])
			if err != nil {
				return err
			}
			formerNames, err := formerNameFlags(cmd)
			if err != nil {
				return err
			}
			failOn, _ := cmd.Flags().GetString("fail-on")
			failOn = strings.ToLower(failOn)
			if failOn != "" && check.RiskRank(failOn) < 1 {
				msg := "--fail-on must be low, medium or high"
				if failOn == check.RiskUnknown {
					msg += ". An unknown risk never fails a check, so read `risk` from --json to catch it"
				}
				return usageError("%s", msg)
			}
			// Find flag mistakes before the check spends any requests.
			if err := checkOutputFlags(f, cmd); err != nil {
				return err
			}
			run, err := runner(f, cmd, services)
			if err != nil {
				return err
			}
			result, err := run.Run(cmd.Context(), name, formerNames...)
			if err != nil {
				return apiError(err)
			}
			if err := writeResult(f, cmd, result); err != nil {
				return err
			}
			if cmd.Context().Err() != nil {
				return interrupted()
			}
			return failOnError(f, result, failOn)
		},
	}
}

// formerNameFlags reads --former-name. Each value is owner/name or a GitHub
// URL. The names only matter for the account check in GH Archive.
func formerNameFlags(cmd *cobra.Command) ([]string, error) {
	values, _ := cmd.Flags().GetStringArray("former-name")
	if noArchive, _ := cmd.Flags().GetBool("no-archive"); noArchive && len(values) > 0 {
		return nil, usageError("--former-name is for the account check in GH Archive, so it does nothing with --no-archive")
	}
	var names []string
	for _, value := range values {
		name, ok := github.ParseName(value)
		if !ok {
			return nil, usageError("--former-name %q is not a GitHub repo; use owner/name, for example snowpackjs/astro", value)
		}
		names = append(names, name)
	}
	return names, nil
}

// writeResult prints a result as text, Markdown or JSON, to the terminal or
// to the --out file.
func writeResult(f *rungrad.Factory, cmd *cobra.Command, result *check.Result) error {
	details, _ := cmd.Flags().GetBool("details")
	markdown, _ := cmd.Flags().GetBool("markdown")
	out, _ := cmd.Flags().GetString("out")
	if err := checkOutputFlags(f, cmd); err != nil {
		return err
	}
	model := result.Summary()
	if details {
		model = *result
	}
	if out == "" {
		if markdown {
			return f.WriteResult(model, func(w io.Writer) { render.Markdown(w, result, details) })
		}
		return f.WriteResult(model, func(w io.Writer) { render.Text(w, result, details) })
	}
	var buf bytes.Buffer
	switch {
	case f.Flags.JSON:
		encoder := json.NewEncoder(&buf)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(model); err != nil {
			return err
		}
	case markdown:
		render.Markdown(&buf, result, details)
	default:
		render.Text(&buf, result, details)
	}
	if err := cache.WriteFile(out, buf.Bytes()); err != nil {
		return rungrad.NewError(rungrad.ExitAPI, fmt.Sprintf("cannot write %s: %v", out, err))
	}
	f.Infof("Wrote %s (risk %s)", out, result.Risk)
	return nil
}

// checkOutputFlags rejects flag combinations that cannot work.
func checkOutputFlags(f *rungrad.Factory, cmd *cobra.Command) error {
	markdown, _ := cmd.Flags().GetBool("markdown")
	if markdown && f.Flags.JSON {
		return usageError("--markdown and --json cannot be used together")
	}
	if out, _ := cmd.Flags().GetString("out"); out != "" {
		if info, err := os.Stat(out); err == nil && info.IsDir() {
			return usageError("--out %s is a folder; give a file name, such as %s", out, filepath.Join(out, "stars.md"))
		}
	}
	return nil
}

// oneArg accepts exactly one argument and otherwise says what to pass.
func oneArg(hint string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return usageError("%s", hint)
		}
		return nil
	}
}

// ExitInterrupted is the usual exit code after Ctrl-C.
const ExitInterrupted = 130

func interrupted() error {
	return rungrad.NewError(ExitInterrupted, "stopped early (Ctrl-C), so the report above is partial and was not saved. Run the same command again for a full check")
}

// failOnError returns the --fail-on exit when the risk reaches the level.
// An unknown risk never does. StarTrust could not judge the repo, so it
// says so instead of failing or passing quietly.
func failOnError(f *rungrad.Factory, result *check.Result, failOn string) error {
	if failOn != "" && result.Risk == check.RiskUnknown {
		f.Infof("StarTrust could not judge %s (risk unknown), so --fail-on %s does not apply.", result.Repository, failOn)
	}
	if failOn == "" || check.RiskRank(result.Risk) < check.RiskRank(failOn) {
		return nil
	}
	return rungrad.NewError(ExitPolicy, fmt.Sprintf("risk is %s for %s (--fail-on %s)", result.Risk, result.Repository, failOn))
}
