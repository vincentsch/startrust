# startrust

Check if a GitHub repo's stars look bought

StarTrust checks whether a GitHub repo's stars look bought. It reads GitHub's daily star counts and marks unusual spikes. Then it checks the accounts behind the busiest days in GH Archive, a public record of GitHub events. Any public repo works, with or without a GitHub token.

## Usage

```
startrust
```

## Examples

```
startrust check junegunn/fzf
startrust check astral-sh/uv --details
startrust compare charmbracelet/glow junegunn/fzf
```

## Shared flags

See [global flags](index.md#global-flags) for shared options.
