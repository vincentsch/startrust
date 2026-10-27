# startrust badge

Make a Shields.io badge from a saved check

Badge turns a saved check into the JSON file that a Shields.io endpoint badge reads. The badge then shows, for example, "stars (experimental): no unusual pattern". Publish the file somewhere public, for example with the GitHub Action, and point https://img.shields.io/endpoint?url=... at it.

## Usage

```
startrust badge <owner/repo | file.json>
```

## Examples

```
startrust badge junegunn/fzf
startrust badge ./check.json --out badge.json
```

## Flags

- `--cache-dir` Folder for saved checks (default: startrust in your user cache directory). Type: `string`. Default: Linux `~/.cache/startrust`, macOS `~/Library/Caches/startrust`, Windows `%LocalAppData%\startrust`.
- `--out` Write the badge JSON to this file. Type: `string`. Default: none.

## Output

Prints text for people by default. Use `--json` for JSON output.

## Related commands

- startrust check
- startrust report

## Shared flags

See [global flags](index.md#global-flags) for shared options.
