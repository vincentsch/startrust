# CI and badge

You can check a repo in GitHub Actions and fail the job on a risk level.
You can also publish a badge with the result. StarTrust is experimental, so
a failing check is a reason to look closer. It does not prove anything.

## The GitHub Action

The action downloads a StarTrust release binary and checks its checksum.
Then it checks the repo and adds the Markdown report with the chart to the
job summary.

```yaml
name: Stars
on:
  schedule:
    - cron: '0 6 * * 1'
  workflow_dispatch:

permissions:
  contents: read

jobs:
  stars:
    runs-on: ubuntu-latest
    steps:
      - uses: vincentsch/startrust@v0.1.0
        with:
          repository: ${{ github.repository }}
```

The action works once StarTrust has its first release. Use a release tag
such as `v0.1.0` after `@`.

Inputs:

| Input | Default | What it does |
| --- | --- | --- |
| `repository` | the current repo | Repo to check, as owner/name. |
| `version` | `latest` | StarTrust release to install, such as `v0.1.0`. |
| `fail-on` | empty | Fail the step when the risk is at least `low`, `medium` or `high`. Empty never fails. |
| `badge` | empty | Also write badge JSON to this path. |
| `args` | empty | Extra flags for `startrust check`, such as `--explain` or `--former-name old-owner/old-name` for a renamed repo. |
| `token` | the job token | GitHub token for the API. The job token is enough for public repos. |

Outputs: `risk`, `verdict`, and `report` (the path of the full JSON report).

When the risk is `unknown`, StarTrust could not judge the repo. `fail-on`
never fails on that, so the step adds a warning to the run instead. If you
want the job to also fail then, a later step can test
`steps.<id>.outputs.risk == 'unknown'`.

The action is a bash script and needs `jq`, which GitHub's hosted runners
have. It is tested on Linux runners.

### Check your dependencies

The same action can check other repos, for example the projects you depend
on:

```yaml
    strategy:
      matrix:
        repo: [junegunn/fzf, charmbracelet/glow]
    steps:
      - uses: vincentsch/startrust@v0.1.0
        with:
          repository: ${{ matrix.repo }}
          fail-on: high
```

## Fail a job without the action

```sh
startrust check owner/repo --fail-on high
```

`--fail-on` takes `low`, `medium` or `high`. When the risk is at that level
or above, StarTrust prints the report as usual and then exits with code 7.
An `unknown` risk never matches, because StarTrust could not judge the
repo and has nothing to fail on. It says so on stderr and exits with 0.
Read `risk` from `--json` if your script should treat that as a failure.
Other problems exit with the codes in the [troubleshooting
guide](troubleshooting.md#exit-codes).

## The badge

`startrust badge` turns a saved check into the JSON that a
[Shields.io endpoint badge](https://shields.io/badges/endpoint-badge) reads:

```sh
startrust check owner/repo
startrust badge owner/repo --out badge.json
```

The badge reads "stars (experimental)" on the left. The right side says
what the check saw:

| Risk | Badge text |
| --- | --- |
| low, with the accounts behind the busy days checked | no unusual pattern |
| low, when the accounts behind some busy days could not be checked | no unusual timing |
| medium, with more one-off accounts than usual | more one-off accounts |
| medium, with unusual spikes | 1 unusual spike, 2 unusual spikes, and so on |
| medium, with a burst only | one big burst |
| high | unusual pattern |
| unknown, for a repo younger than 240 days | too new to judge |
| unknown, for other reasons | not enough data |
| unknown, for a repo without stars | no stars yet |

The badge shows the same words as the `headline` field of the JSON. When
the accounts behind some busy days could not be checked, it says "no
unusual timing" instead of "no unusual pattern". When nothing could be
judged, it says so.

Every badge has the same neutral color. A green or red badge would claim
more certainty than the calibration supports today.

Shields.io needs to fetch the JSON from a public URL. One way is a weekly
job that lets the action write the badge and commits it when it changed:

```yaml
name: Stars badge
on:
  schedule:
    - cron: '0 6 * * 1'
  workflow_dispatch:

permissions:
  contents: write

jobs:
  badge:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
      - uses: vincentsch/startrust@v0.1.0
        with:
          badge: badge/stars.json
      - name: Commit the badge when it changed
        run: |
          git add badge/stars.json
          git diff --cached --quiet && exit 0
          git config user.name 'github-actions[bot]'
          git config user.email '41898282+github-actions[bot]@users.noreply.github.com'
          git commit -m 'Update the stars badge'
          git push
```

The action creates the `badge` folder if it is missing. Then use this in
your README, with the raw URL of the file:

```markdown
![stars](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/owner/repo/main/badge/stars.json)
```

Anyone can write a badge file by hand, so a badge alone proves nothing.
Readers who want to be sure should run `startrust check` themselves.
