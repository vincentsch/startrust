package cli

import (
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"
	rungrad "github.com/vincentsch/rungrad"

	"github.com/vincentsch/startrust/internal/cache"
	"github.com/vincentsch/startrust/internal/check"
)

func cacheCommand(services Services) *rungrad.Command {
	root := &rungrad.Command{
		Use:   "cache",
		Short: "List or prune saved checks",
		Long:  "StarTrust saves every finished check, so `report` and `badge` work offline. It also saves\nthe GH Archive answers for past days, because they never change.",
	}
	root.AddCommand(cacheListCommand(), cachePruneCommand())
	return root
}

type cacheList struct {
	Directory string        `json:"directory"`
	Checks    []cache.Entry `json:"checks"`
}

func cacheListCommand() *rungrad.Command {
	return &rungrad.Command{
		Use:         "list [owner/repo]",
		Short:       "List saved checks, newest first",
		Args:        cobra.MaximumNArgs(1),
		Examples:    []string{"startrust cache list", "startrust cache list junegunn/fzf --json"},
		OutputModes: []string{rungrad.OutputModeHuman, rungrad.OutputModeJSON},
		Related:     []string{"startrust report", "startrust cache prune"},
		Configure:   addCacheFlag,
		Run: func(f *rungrad.Factory, cmd *cobra.Command, args []string) error {
			repo := ""
			if len(args) == 1 {
				name, err := parseRepo(args[0])
				if err != nil {
					return err
				}
				repo = name
			}
			store, err := openCache(cmd)
			if err != nil {
				return err
			}
			entries, err := store.Checks(repo)
			if err != nil {
				return rungrad.NewError(rungrad.ExitAPI, fmt.Sprintf("cannot read the cache: %v", err))
			}
			list := cacheList{Directory: store.Root, Checks: entries}
			if list.Checks == nil {
				list.Checks = []cache.Entry{}
			}
			return f.WriteResult(list, func(w io.Writer) {
				fmt.Fprintf(w, "Cache: %s\n\n", store.Root)
				if len(entries) == 0 {
					fmt.Fprintln(w, "No saved checks yet. Run `startrust check owner/repo`.")
					return
				}
				width := 0
				for _, e := range entries {
					width = max(width, len(e.Repository))
				}
				for _, e := range entries {
					fmt.Fprintf(w, "%-*s  %s  %s\n", width, e.Repository, e.CheckedAt.Local().Format("2006-01-02 15:04"), size(e.Bytes))
				}
			})
		},
	}
}

func cachePruneCommand() *rungrad.Command {
	return &rungrad.Command{
		Use:   "prune",
		Short: "Delete old checks, but not the newest check of each repo",
		Long: "Prune deletes saved checks that are older than --older-than, but never the newest check\n" +
			"of a repo. It also deletes GH Archive answers and unfinished --deep stargazer lists saved\n" +
			"before that time.",
		Args: cobra.NoArgs,
		Examples: []string{
			"startrust cache prune --dry-run",
			"startrust cache prune --older-than 168h --confirm",
		},
		Destructive: true,
		OutputModes: []string{rungrad.OutputModeHuman, rungrad.OutputModeJSON},
		Related:     []string{"startrust cache list"},
		Configure: func(cmd *cobra.Command) {
			cmd.Flags().Duration("older-than", 30*24*time.Hour, "Delete files older than this")
			cmd.Flags().Bool("confirm", false, "Delete without asking")
			addCacheFlag(cmd)
		},
		Run: func(f *rungrad.Factory, cmd *cobra.Command, args []string) error {
			age, _ := cmd.Flags().GetDuration("older-than")
			if age <= 0 {
				return usageError("--older-than must be positive, for example 720h")
			}
			store, err := openCache(cmd)
			if err != nil {
				return err
			}
			cutoff := time.Now().Add(-age)
			if !f.DryRun() {
				preview, err := store.Prune(cutoff, true)
				if err != nil {
					return rungrad.NewError(rungrad.ExitAPI, err.Error())
				}
				// Ask even when there is nothing to delete, so a script gets
				// the same answer whatever the cache holds.
				confirmed, _ := cmd.Flags().GetBool("confirm")
				target := fmt.Sprintf("%s (%s) in %s", count(preview.Files, "file", "files"), size(preview.Bytes), store.Root)
				if err := f.ConfirmDestructive(rungrad.ConfirmOptions{Action: "delete", Target: target, Confirmed: confirmed}); err != nil {
					return err
				}
			}
			plan, err := store.Prune(cutoff, f.DryRun())
			if err != nil {
				return rungrad.NewError(rungrad.ExitAPI, err.Error())
			}
			return f.WriteResult(plan, func(w io.Writer) {
				if plan.Files == 0 {
					if plan.DryRun {
						fmt.Fprintln(w, "Dry run: nothing would be deleted.")
					} else {
						fmt.Fprintln(w, "Nothing to prune.")
					}
					return
				}
				verb := "Deleted"
				if plan.DryRun {
					verb = "Would delete"
				}
				fmt.Fprintf(w, "%s %s (%s). Kept %s, including the newest check of each repo.\n", verb,
					count(plan.Files, "file", "files"), size(plan.Bytes), count(plan.Kept, "check", "checks"))
				if plan.DryRun {
					for _, item := range plan.Items {
						fmt.Fprintf(w, "  %s\n", item.Path)
					}
				}
			})
		},
	}
}

// count writes n with the right noun, such as "1 file" or "3 files".
func count(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return check.Number(n) + " " + many
}

func size(bytes int64) string {
	switch {
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(bytes)/(1<<10))
	}
	return check.Number(int(bytes)) + " bytes"
}
