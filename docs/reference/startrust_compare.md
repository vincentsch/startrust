# startrust compare

Check several repos and show them side by side

Compare checks two to eight repos and shows their risk and biggest day in one table. Below the table it draws their star histories on one timeline and prints each verdict.

## Usage

```
startrust compare <owner/repo> <owner/repo>...
```

## Examples

```
startrust compare junegunn/fzf charmbracelet/glow
startrust compare astral-sh/uv junegunn/fzf --no-archive
startrust compare a/one b/two --json
```

## Flags

- `--cache-dir` Folder for saved checks (default: startrust in your user cache directory). Type: `string`. Default: Linux `~/.cache/startrust`, macOS `~/Library/Caches/startrust`, Windows `%LocalAppData%\startrust`.
- `--details` Include the full evidence in --json. Type: `bool`. Default: `false`.
- `--explain` Look for Hacker News posts and releases near the busiest days. Type: `bool`. Default: `false`.
- `--max-requests` Stop after this many GitHub requests for all repos together and report what was read. Type: `int`. Default: `1000`.
- `--no-archive` Leave out the account check in GH Archive (the verdict then rests on the timing of the stars). Type: `bool`. Default: `false`.
- `--no-token` Check without a GitHub token, even when one is set or you are logged in with the GitHub CLI. Type: `bool`. Default: `false`.

## Output

Prints text for people by default. Use `--json` for JSON output.

## Related commands

- startrust check

## Shared flags

See [global flags](index.md#global-flags) for shared options.
