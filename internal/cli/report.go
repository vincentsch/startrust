package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/spf13/cobra"
	rungrad "github.com/vincentsch/rungrad"

	"github.com/vincentsch/startrust/internal/check"
)

func reportCommand(services Services) *rungrad.Command {
	return &rungrad.Command{
		Use:   "report <owner/repo | file.json>",
		Short: "Show a saved check again",
		Long: "Report shows a saved check without asking GitHub again. Give it a repo to show that\n" +
			"repo's newest check from the cache, or a JSON file saved with\n" +
			"`check --json --details --out`. If a file with that name exists, report reads the file\n" +
			"and does not look for a repo.",
		Args: oneArg("report takes one repo or one saved JSON file, such as junegunn/fzf or ./check.json"),
		Examples: []string{
			"startrust report junegunn/fzf",
			"startrust report junegunn/fzf --markdown --out fzf-stars.md",
			"startrust report ./check.json --details",
		},
		OutputModes: []string{rungrad.OutputModeHuman, rungrad.OutputModeJSON},
		Related:     []string{"startrust check", "startrust cache list"},
		Configure: func(cmd *cobra.Command) {
			cmd.Flags().Bool("details", false, "Show the full evidence (and include it in --json)")
			cmd.Flags().Bool("markdown", false, "Write a Markdown report for sharing")
			cmd.Flags().String("out", "", "Write the report to this file instead of the terminal")
			addCacheFlag(cmd)
		},
		Run: func(f *rungrad.Factory, cmd *cobra.Command, args []string) error {
			if err := checkOutputFlags(f, cmd); err != nil {
				return err
			}
			result, err := loadResult(cmd, args[0])
			if err != nil {
				return err
			}
			return writeResult(f, cmd, result)
		},
	}
}

// loadResult reads a check from a file, or the newest cached check of a
// repo. A path that exists is always read as a file, so out/report.json is
// never mistaken for a repo called "out/report.json".
func loadResult(cmd *cobra.Command, arg string) (*check.Result, error) {
	if info, err := os.Stat(arg); err == nil && !info.IsDir() {
		data, err := os.ReadFile(arg)
		if err != nil {
			return nil, rungrad.NewError(rungrad.ExitAPI, fmt.Sprintf("cannot read %s: %v", arg, err))
		}
		return decodeResult(data, arg)
	}
	looksLikeFile := strings.HasSuffix(strings.ToLower(arg), ".json") || strings.HasPrefix(arg, ".") || strings.ContainsAny(arg, `\`)
	name, ok := parseRepoQuiet(arg)
	if !ok || looksLikeFile {
		return nil, rungrad.NewError(rungrad.ExitNotFound, fmt.Sprintf("no file called %s", arg))
	}
	store, err := openCache(cmd)
	if err != nil {
		return nil, err
	}
	data, entry, err := store.LatestCheck(name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, rungrad.NewError(rungrad.ExitNotFound, fmt.Sprintf("no saved check for %s; run `startrust check %s` first", name, name))
	}
	if err != nil {
		return nil, rungrad.NewError(rungrad.ExitAPI, fmt.Sprintf("cannot read the cache: %v", err))
	}
	return decodeResult(data, entry.Path)
}

func decodeResult(data []byte, source string) (*check.Result, error) {
	var result check.Result
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, usageError("%s is not a StarTrust check: %v", source, err)
	}
	// A check without a headline comes from an older build, which judged
	// some repos differently; showing it would mix two sets of rules.
	if result.SchemaVersion != check.SchemaVersion || result.Repository == "" || check.RiskRank(result.Risk) < 0 || result.Headline == "" {
		return nil, usageError("%s is not a StarTrust check (or comes from an incompatible version)", source)
	}
	return &result, nil
}

func parseRepoQuiet(arg string) (string, bool) {
	name, err := parseRepo(arg)
	return name, err == nil
}
