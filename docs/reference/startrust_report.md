# startrust report

Show a saved check again

Report shows a saved check without asking GitHub again. Give it a repo to show that repo's newest check from the cache, or a JSON file saved with `check --json --details --out`. If a file with that name exists, report reads the file and does not look for a repo.

## Usage

```
startrust report <owner/repo | file.json>
```

## Examples

```
startrust report junegunn/fzf
startrust report junegunn/fzf --markdown --out fzf-stars.md
startrust report ./check.json --details
```

## Flags

- `--cache-dir` Folder for saved checks (default: startrust in your user cache directory). Type: `string`. Default: Linux `~/.cache/startrust`, macOS `~/Library/Caches/startrust`, Windows `%LocalAppData%\startrust`.
- `--details` Show the full evidence (and include it in --json). Type: `bool`. Default: `false`.
- `--markdown` Write a Markdown report for sharing. Type: `bool`. Default: `false`.
- `--out` Write the report to this file instead of the terminal. Type: `string`. Default: none.

## Output

Prints text for people by default. Use `--json` for JSON output.

## Related commands

- startrust check
- startrust cache list

## Shared flags

See [global flags](index.md#global-flags) for shared options.
