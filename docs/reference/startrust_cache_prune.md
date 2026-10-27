# startrust cache prune

Delete old checks, but not the newest check of each repo

Prune deletes saved checks that are older than --older-than, but never the newest check of a repo. It also deletes GH Archive answers and unfinished --deep stargazer lists saved before that time.

## Usage

```
startrust cache prune
```

## Examples

```
startrust cache prune --dry-run
startrust cache prune --older-than 168h --confirm
```

## Flags

- `--cache-dir` Folder for saved checks (default: startrust in your user cache directory). Type: `string`. Default: Linux `~/.cache/startrust`, macOS `~/Library/Caches/startrust`, Windows `%LocalAppData%\startrust`.
- `--confirm` Delete without asking. Type: `bool`. Default: `false`.
- `--older-than` Delete files older than this. Type: `duration`. Default: `720h0m0s`.

## Output

Prints text for people by default. Use `--json` for JSON output.

## Preview changes

This command makes changes. Use `--dry-run` to see what would change without changing anything.

## Destructive

This command performs a destructive action and asks for confirmation before acting. Preview it first with `--dry-run`; outside a dry run it proceeds only after explicit confirmation, and in non-interactive mode (`--json`, `--no-prompt`, or no terminal) it requires a confirmation flag instead of blocking.

## Related commands

- startrust cache list

## Shared flags

See [global flags](index.md#global-flags) for shared options.
