package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	rungrad "github.com/vincentsch/rungrad"

	"github.com/vincentsch/startrust/internal/check"
	"github.com/vincentsch/startrust/internal/render"
)

type comparison struct {
	Repositories []check.Result `json:"repositories"`
	Failed       []failedCheck  `json:"failed"`
}

type failedCheck struct {
	Repository string `json:"repository"`
	Error      string `json:"error"`
}

func compareCommand(services Services) *rungrad.Command {
	return &rungrad.Command{
		Use:   "compare <owner/repo> <owner/repo>...",
		Short: "Check several repos and show them side by side",
		Long: "Compare checks two to eight repos and shows their risk and biggest day in one table.\n" +
			"Below the table it draws their star histories on one timeline and prints each verdict.\n\n" +
			"If one of the repos was renamed, add its old name with --former-name, written as\n" +
			"repo=old-owner/old-name. GitHub must forward the old name to the repo, the same as for\n" +
			"`startrust check --former-name`.",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) < 2 || len(args) > 8 {
				return usageError("compare takes 2 to 8 repos; use `startrust check` for one")
			}
			return nil
		},
		Examples: []string{
			"startrust compare junegunn/fzf charmbracelet/glow",
			"startrust compare astral-sh/uv junegunn/fzf --no-archive",
			"startrust compare a/one b/two --json",
			"startrust compare sindresorhus/is withastro/astro --former-name withastro/astro=snowpackjs/astro",
		},
		OutputModes: []string{rungrad.OutputModeHuman, rungrad.OutputModeJSON},
		Related:     []string{"startrust check"},
		Configure: func(cmd *cobra.Command) {
			cmd.Flags().Bool("details", false, "Include the full evidence in --json")
			cmd.Flags().StringArray("former-name", nil, "An earlier name of one of the repos, written as `repo=old`, such as withastro/astro=snowpackjs/astro (repeat it for several)")
			addRunFlags(cmd)
			// One GitHub client checks all repos, so the limit is for all of them.
			cmd.Flags().Lookup("max-requests").Usage = "Stop after this many GitHub requests for all repos together and report what was read"
		},
		Run: func(f *rungrad.Factory, cmd *cobra.Command, args []string) error {
			var names []string
			for _, arg := range args {
				name, err := parseRepo(arg)
				if err != nil {
					return err
				}
				names = append(names, name)
			}
			formerNames, err := compareFormerNames(cmd, names)
			if err != nil {
				return err
			}
			run, err := runner(f, cmd, services)
			if err != nil {
				return err
			}
			details, _ := cmd.Flags().GetBool("details")
			out := comparison{Repositories: []check.Result{}, Failed: []failedCheck{}}
			var results []*check.Result
			var firstErr error
			for _, name := range names {
				if cmd.Context().Err() != nil {
					break
				}
				result, err := run.Run(cmd.Context(), name, formerNames[strings.ToLower(name)]...)
				if err != nil {
					if cmd.Context().Err() != nil {
						break
					}
					out.Failed = append(out.Failed, failedCheck{Repository: name, Error: err.Error()})
					if firstErr == nil {
						firstErr = err
					}
					continue
				}
				results = append(results, result)
				if details {
					out.Repositories = append(out.Repositories, *result)
				} else {
					out.Repositories = append(out.Repositories, result.Summary())
				}
			}
			if len(results) == 0 {
				if cmd.Context().Err() != nil {
					return rungrad.NewError(ExitInterrupted, "stopped (Ctrl-C) before any repo was checked")
				}
				return apiError(firstErr)
			}
			err = f.WriteResult(out, func(w io.Writer) {
				render.Compare(w, results)
				for _, failed := range out.Failed {
					fmt.Fprintf(w, "\n%s\n", render.Wrap(failed.Repository+" could not be checked: "+failed.Error, 80, "  "))
				}
			})
			switch {
			case err != nil:
				return err
			case cmd.Context().Err() != nil:
				return interrupted()
			case firstErr != nil:
				// The table shows the repos that worked; the exit code
				// still tells a script that one did not.
				return apiError(firstErr)
			}
			return nil
		},
	}
}

// compareFormerNames reads compare's --former-name values, each written as
// repo=old-owner/old-name, and returns the former names of each repo, keyed
// by its lowercased name. The repo must be one of those to compare.
func compareFormerNames(cmd *cobra.Command, repos []string) (map[string][]string, error) {
	values, err := formerNameValues(cmd)
	if err != nil {
		return nil, err
	}
	byRepo := map[string][]string{}
	for _, value := range values {
		repoPart, oldPart, ok := strings.Cut(value, "=")
		if !ok {
			return nil, usageError("--former-name %q needs the repo and its old name, such as withastro/astro=snowpackjs/astro", value)
		}
		repo, err := parseRepo(repoPart)
		if err != nil {
			return nil, err
		}
		old, err := parseFormerName(oldPart)
		if err != nil {
			return nil, err
		}
		known := false
		for _, name := range repos {
			known = known || strings.EqualFold(name, repo)
		}
		if !known {
			return nil, usageError("--former-name %s: %s is not one of the repos to compare", value, repo)
		}
		key := strings.ToLower(repo)
		byRepo[key] = append(byRepo[key], old)
	}
	return byRepo, nil
}
