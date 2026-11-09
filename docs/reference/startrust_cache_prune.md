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

This command changes things. Add `--dry-run` to see what it would change; nothing changes then.

## Confirmation

This command deletes or overwrites something, so it asks before it acts. Preview it with `--dry-run` first. It doesn't ask when there is no terminal or when you pass `--json` or `--no-prompt`; then it needs its confirmation flag.

## Related commands

- startrust cache list

## Shared flags

See [global flags](index.md#global-flags) for shared options.
