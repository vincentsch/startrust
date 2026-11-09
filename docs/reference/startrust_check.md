# startrust check

Check whether a repo's stars look bought

Check reads the repo's daily star counts from GitHub and draws them as a chart. It marks unusual spikes and looks up the accounts behind the busiest days in GH Archive, a public record of GitHub events. It prints the verdict first, then the evidence.

Below the verdict you see the repo's activity, such as its forks and recent issues. They help when a repo is too new for StarTrust to judge, but they never change the risk.

You need no setup. Without a token, GitHub allows 60 requests an hour. A check uses about one request for every 30 weeks of the repo's age, plus two, so a ten-year-old repo needs about 20. If GITHUB_TOKEN or GH_TOKEN is set, or you are logged in with the GitHub CLI, StarTrust uses that token. The last lines of the report say which one it used and how many requests were left. Add --no-token to check without one.

StarTrust changes nothing on GitHub. It saves each check in its cache folder, so `report` and `badge` can show it later. `startrust cache list` shows where that folder is, and --cache-dir picks another one.

GH Archive files stars under the name a repo had at the time. If the repo was renamed, add its old name with --former-name, and one check covers its whole history. That is a new full check, with as many requests as the first one and one more for each name. Checking the old name itself works too, because GitHub forwards it to the repo. Without the old name the check stays partial, because the accounts behind the busy days from that time cannot be checked.

Maintainers can add --deep to compare the profiles of the people who starred their repo.

## Usage

```
startrust check <owner/repo>
```

## Examples

```
startrust check junegunn/fzf
startrust check https://github.com/charmbracelet/glow --details
startrust check owner/repo --markdown --out stars.md
startrust check owner/repo --json
startrust check owner/repo --fail-on high
startrust check owner/repo --no-token
startrust check withastro/astro --former-name snowpackjs/astro
```

## Flags

- `--cache-dir` Folder for saved checks (default: startrust in your user cache directory). Type: `string`. Default: Linux `~/.cache/startrust`, macOS `~/Library/Caches/startrust`, Windows `%LocalAppData%\startrust`.
- `--deep` Also list who starred and compare their profiles (needs push access to the repo). Type: `bool`. Default: `false`.
- `--details` Show the full evidence (and include it in --json). Type: `bool`. Default: `false`.
- `--explain` Look for Hacker News posts and releases near the busiest days. Type: `bool`. Default: `false`.
- `--fail-on` Exit with code 7 when the risk is at least this level: low, medium or high (an unknown risk never matches). Type: `string`. Default: none.
- `--former-name` An earlier `owner/name` of the repo, also looked up in GH Archive (repeat it for several). Type: `string`. Default: none.
- `--markdown` Write a Markdown report for sharing. Type: `bool`. Default: `false`.
- `--max-requests` Stop after this many GitHub requests and report what was read. Type: `int`. Default: `1000`.
- `--no-archive` Leave out the account check in GH Archive (the verdict then rests on the timing of the stars). Type: `bool`. Default: `false`.
- `--no-token` Check without a GitHub token, even when one is set or you are logged in with the GitHub CLI. Type: `bool`. Default: `false`.
- `--out` Write the report to this file instead of the terminal. Type: `string`. Default: none.

## Output

Prints text for people by default. Use `--json` for JSON output.

## Related commands

- startrust compare
- startrust report
- startrust badge

## Shared flags

See [global flags](index.md#global-flags) for shared options.
