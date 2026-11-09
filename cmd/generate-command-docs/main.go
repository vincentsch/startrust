// Command generate-command-docs writes or checks docs/reference, the command
// reference generated from the CLI's own help.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/vincentsch/rungrad/docsgen"

	"github.com/vincentsch/startrust/internal/cli"
)

// generate builds every reference page, with each flag's type and default
// added to rungrad's text.
func generate() map[string]string {
	app := cli.NewApp(cli.Services{})
	pages := docsgen.Generate(app)
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		path := strings.ReplaceAll(cmd.CommandPath(), " ", "_") + ".md"
		if page, ok := pages[path]; ok {
			page = strings.ReplaceAll(page, "Shows text you can read in your terminal.\n\nUse --json to get JSON for scripts and other programs.",
				"Prints text for people by default. Use `--json` for JSON output.")
			if cmd.Long != "" {
				// Put the long description under the one-line summary,
				// joined into paragraphs instead of terminal-width lines.
				var paragraphs []string
				for _, p := range strings.Split(cmd.Long, "\n\n") {
					// rungrad appends its own sections to the help text;
					// the reference page has them already.
					if strings.HasPrefix(p, "Related commands") || strings.HasPrefix(p, "Output") {
						break
					}
					paragraphs = append(paragraphs, strings.Join(strings.Fields(p), " "))
				}
				page = strings.Replace(page, cmd.Short+"\n", cmd.Short+"\n\n"+strings.Join(paragraphs, "\n\n")+"\n", 1)
			}
			cmd.LocalFlags().VisitAll(func(f *pflag.Flag) { page = describeFlag(page, f) })
			pages[path] = strings.TrimRight(page, "\n") + "\n\n## Shared flags\n\nSee [global flags](index.md#global-flags) for shared options.\n"
		}
		for _, child := range cmd.Commands() {
			walk(child)
		}
	}
	walk(app.Root())
	app.Root().PersistentFlags().VisitAll(func(f *pflag.Flag) { pages["index.md"] = describeFlag(pages["index.md"], f) })
	pages["index.md"] = strings.ReplaceAll(pages["index.md"], ".md) - ", ".md): ")
	return pages
}

func describeFlag(page string, f *pflag.Flag) string {
	prefix := "- `--" + f.Name + "` " + f.Usage
	def := "`" + f.DefValue + "`"
	switch {
	case f.Name == "cache-dir":
		def = "Linux `~/.cache/startrust`, macOS `~/Library/Caches/startrust`, Windows `%LocalAppData%\\startrust`"
	case f.DefValue == "" || f.DefValue == "[]":
		def = "none"
	}
	// A flag you can repeat says so in its text, so its type is just a
	// string.
	kind := strings.TrimSuffix(f.Value.Type(), "Array")
	return strings.ReplaceAll(page, prefix+"\n", prefix+". Type: `"+kind+"`. Default: "+def+".\n")
}

func run(check bool, dir string) error {
	want := generate()
	result := docsgen.CheckResult{}
	for path, content := range want {
		got, err := os.ReadFile(filepath.Join(dir, path))
		switch {
		case errors.Is(err, os.ErrNotExist):
			result.Missing = append(result.Missing, path)
		case err != nil:
			return err
		case string(got) != content:
			result.Stale = append(result.Stale, path)
		}
	}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || !entry.Type().IsRegular() {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if _, ok := want[filepath.ToSlash(rel)]; !ok {
			result.Orphaned = append(result.Orphaned, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	sort.Strings(result.Missing)
	sort.Strings(result.Stale)
	sort.Strings(result.Orphaned)
	if check {
		if !result.OK() {
			return fmt.Errorf("command reference is out of date (missing: %v; stale: %v; extra: %v); run make docs", result.Missing, result.Stale, result.Orphaned)
		}
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for path, content := range want {
		if err := os.WriteFile(filepath.Join(dir, path), []byte(content), 0o644); err != nil {
			return err
		}
	}
	for _, path := range result.Orphaned {
		// Pages of commands that no longer exist.
		if err := os.Remove(filepath.Join(dir, path)); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	check := flag.Bool("check", false, "check the reference without changing it")
	flag.Parse()
	if err := run(*check, filepath.Join("docs", "reference")); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
