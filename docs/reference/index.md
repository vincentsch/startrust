# startrust command reference

Check if a GitHub repo's stars look bought

## Commands

- [startrust badge](startrust_badge.md): Make a Shields.io badge from a saved check
- [startrust cache](startrust_cache.md): List or prune saved checks
- [startrust cache list](startrust_cache_list.md): List saved checks, newest first
- [startrust cache prune](startrust_cache_prune.md): Delete old checks, but not the newest check of each repo
- [startrust check](startrust_check.md): Check whether a repo's stars look bought
- [startrust compare](startrust_compare.md): Check several repos and show them side by side
- [startrust report](startrust_report.md): Show a saved check again

## Global flags

- `--config` Path to the config file. Type: `string`. Default: none.
- `--dry-run` Preview changes without performing them. Type: `bool`. Default: `false`.
- `--json` Output stable JSON instead of the human view. Type: `bool`. Default: `false`.
- `--no-prompt` Never block on an interactive prompt. Type: `bool`. Default: `false`.
- `--quiet` Suppress non-essential output. Type: `bool`. Default: `false`.
