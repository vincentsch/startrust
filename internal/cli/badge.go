package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"
	rungrad "github.com/vincentsch/rungrad"

	"github.com/vincentsch/startrust/internal/cache"
	"github.com/vincentsch/startrust/internal/check"
)

// Badge is a Shields.io endpoint badge.
// See https://shields.io/badges/endpoint-badge.
type Badge struct {
	SchemaVersion int    `json:"schemaVersion"`
	Label         string `json:"label"`
	Message       string `json:"message"`
	Color         string `json:"color"`
}

// BadgeFor describes a result in a few words, using its headline, such as
// "no unusual pattern" or "too new to judge". The color is the same for
// every result. The scoring is still experimental, so the badge reports what
// was seen without a green or red judgement.
func BadgeFor(r *check.Result) Badge {
	return Badge{SchemaVersion: 1, Label: "stars (experimental)", Message: r.Headline, Color: "informational"}
}

func badgeCommand(services Services) *rungrad.Command {
	return &rungrad.Command{
		Use:   "badge <owner/repo | file.json>",
		Short: "Make a Shields.io badge from a saved check",
		Long: "Badge turns a saved check into the JSON file that a Shields.io endpoint badge reads.\n" +
			"The badge then shows, for example, \"stars (experimental): no unusual pattern\". Publish\n" +
			"the file somewhere public, for example with the GitHub Action, and point\n" +
			"https://img.shields.io/endpoint?url=... at it.",
		Args: oneArg("badge takes one repo or one saved JSON file, such as junegunn/fzf or ./check.json"),
		Examples: []string{
			"startrust badge junegunn/fzf",
			"startrust badge ./check.json --out badge.json",
		},
		OutputModes: []string{rungrad.OutputModeHuman, rungrad.OutputModeJSON},
		Related:     []string{"startrust check", "startrust report"},
		Configure: func(cmd *cobra.Command) {
			cmd.Flags().String("out", "", "Write the badge JSON to this file")
			addCacheFlag(cmd)
		},
		Run: func(f *rungrad.Factory, cmd *cobra.Command, args []string) error {
			result, err := loadResult(cmd, args[0])
			if err != nil {
				return err
			}
			badge := BadgeFor(result)
			out, _ := cmd.Flags().GetString("out")
			if out != "" {
				var buf bytes.Buffer
				encoder := json.NewEncoder(&buf)
				encoder.SetIndent("", "  ")
				_ = encoder.Encode(badge)
				if err := cache.WriteFile(out, buf.Bytes()); err != nil {
					return rungrad.NewError(rungrad.ExitAPI, fmt.Sprintf("cannot write %s: %v", out, err))
				}
				f.Infof("Wrote %s: %q", out, badge.Label+": "+badge.Message)
				return nil
			}
			return f.WriteResult(badge, func(w io.Writer) {
				encoder := json.NewEncoder(w)
				encoder.SetIndent("", "  ")
				_ = encoder.Encode(badge)
			})
		},
	}
}
