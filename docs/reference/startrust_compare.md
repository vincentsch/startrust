# startrust compare

Check several repos and show them side by side

Compare checks two to eight repos and shows their risk and biggest day in one table. Below the table it draws their star histories on one timeline and prints each verdict.

If one of the repos was renamed, add its old name with --former-name, written as repo=old-owner/old-name. GitHub must forward the old name to the repo, the same as for `startrust check --former-name`.

## Usage

```
startrust compare <owner/repo> <owner/repo>...
```

## Examples

```
startrust compare junegunn/fzf charmbracelet/glow
startrust compare astral-sh/uv junegunn/fzf --no-archive
startrust compare a/one b/two --json
startrust compare sindresorhus/is withastro/astro --former-name withastro/astro=snowpackjs/astro
```

## Flags

- `--cache-dir` Folder for saved checks (default: startrust in your user cache directory). Type: `string`. Default: Linux `~/.cache/startrust`, macOS `~/Library/Caches/startrust`, Windows `%LocalAppData%\startrust`.
- `--details` Include the full evidence in --json. Type: `bool`. Default: `false`.
- `--explain` Look for Hacker News posts and releases near the busiest days. Type: `bool`. Default: `false`.
- `--former-name` An earlier name of one of the repos, written as `repo=old`, such as withastro/astro=snowpackjs/astro (repeat it for several). Type: `string`. Default: none.
- `--max-requests` Stop after this many GitHub requests for all repos together and report what was read. Type: `int`. Default: `1000`.
- `--no-archive` Leave out the account check in GH Archive (the verdict then rests on the timing of the stars). Type: `bool`. Default: `false`.
- `--no-token` Check without a GitHub token, even when one is set or you are logged in with the GitHub CLI. Type: `bool`. Default: `false`.

## Output

Prints text for people by default. Use `--json` for JSON output.

## Related commands

- startrust check

## Shared flags

See [global flags](index.md#global-flags) for shared options.
