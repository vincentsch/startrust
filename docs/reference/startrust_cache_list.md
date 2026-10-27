# startrust cache list

List saved checks, newest first

List saved checks, newest first

## Usage

```
startrust cache list [owner/repo]
```

## Examples

```
startrust cache list
startrust cache list junegunn/fzf --json
```

## Flags

- `--cache-dir` Folder for saved checks (default: startrust in your user cache directory). Type: `string`. Default: Linux `~/.cache/startrust`, macOS `~/Library/Caches/startrust`, Windows `%LocalAppData%\startrust`.

## Output

Prints text for people by default. Use `--json` for JSON output.

## Related commands

- startrust report
- startrust cache prune

## Shared flags

See [global flags](index.md#global-flags) for shared options.
