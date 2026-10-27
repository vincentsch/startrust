package cli

import (
	"fmt"
	"io"

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
			"Below the table it draws their star histories on one timeline and prints each verdict.",
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
		},
		OutputModes: []string{rungrad.OutputModeHuman, rungrad.OutputModeJSON},
		Related:     []string{"startrust check"},
		Configure: func(cmd *cobra.Command) {
			cmd.Flags().Bool("details", false, "Include the full evidence in --json")
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
				result, err := run.Run(cmd.Context(), name)
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
