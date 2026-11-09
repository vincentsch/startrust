# StarTrust for coding agents

StarTrust checks whether a GitHub repo's stars look bought. It reports what
the star history and the accounts behind the busiest days look like. It
changes nothing on GitHub. On this computer it only writes its cache
folder, where it saves each check (see [step 2](#2-run-the-check)). Follow
these steps when a user asks you to check a repo. They also cover
installing StarTrust and adding it to CI.

The setup prompt from the README is:

```text
Install StarTrust from https://github.com/vincentsch/startrust (read its
AGENTS.md and follow the steps), then check whether the stars of
<owner/repo> look bought and explain the result to me.
```

## 1. Check that StarTrust is installed

Run `startrust --version`. Only a "command not found" result means it is
missing. If it fails in another way, report the error instead of
reinstalling over it.

To install on Linux and macOS, or on Windows in Git Bash:

```sh
curl -fsSL https://raw.githubusercontent.com/vincentsch/startrust/main/install.sh | sh
```

The installer checks the SHA-256 checksum and installs to `~/.local/bin`
(or `INSTALL_DIR` when the user set it). If it prints a PATH line, add that
directory to PATH for this session only. Do not edit shell profiles unless
the user asks.

On native Windows, download `startrust-windows-amd64.exe` (or `-arm64.exe`)
and `checksums.txt` from the latest GitHub release. Compare the output of
`Get-FileHash -Algorithm SHA256` with the matching line before you run it.
With Go 1.27 or newer, `go install github.com/vincentsch/startrust/cmd/startrust@latest`
also works.

Run `startrust --version` again and check where the binary is
(`command -v startrust`, or `(Get-Command startrust).Source` in PowerShell).

## 2. Run the check

```sh
startrust check owner/repo --json
```

The repo can also be a GitHub URL. The command needs no token. It uses a
token from `GITHUB_TOKEN` or `GH_TOKEN`, or the GitHub CLI login, when one
is there. Never ask the user to paste a token into the chat, and never
print one.

`sources.token_source` in the JSON says which token the check used:
`GITHUB_TOKEN`, `GH_TOKEN`, `gh` or `none`. `gh` means StarTrust got it from
`gh auth token`. The GitHub CLI can find a login in the system keyring even
when `gh auth status` reports none, for example when HOME points to an
empty folder. If the user wants a check without any token, add
`--no-token`. StarTrust reads no config file, so these are the only places
a token can come from.

Without a token GitHub allows 60 requests an hour. A check uses about one
request for every 30 weeks of the repo's age, plus two. So a five-year-old
repo needs about 11, and a ten-year-old repo about 20. The number of stars
does not matter. Each `--former-name` adds one. `sources.github_requests`
says how many the check used. `sources.github_rate_limit` has GitHub's own
numbers after the check. Its `remaining` says how many requests were left,
and `resets_at` says when the limit starts again. Look at `remaining`
before you run more checks. If the command exits with code 6, the limit is
used up. Tell the user when it resets (the message says) and that `gh auth
login` raises it.

Each check saves its result in StarTrust's cache folder, so `startrust
report owner/repo` can show it again without new requests. `startrust
cache list` shows where the folder is. To keep everything in the user's
current folder instead, add `--cache-dir ./startrust-cache` to every
command.

Read these fields from the JSON, and only these:

- `risk`: `low`, `medium`, `high` or `unknown`.
- `headline`: the result in a few words, such as `no unusual pattern` or
  `too new to judge`.
- `verdict`: a few sentences that describe what the check saw. Quote it.
- `accounts_checked`: `all`, `some` or `none`. With `none`, the verdict
  rests on the timing of the stars.
- `days_checked` and `days_to_check`: on how many of the busy days that
  needed it the account check counts. Always give them with the risk, as
  the report does, such as "low, accounts checked on 1 of 3 busy days".
- `rename_suspected`: `true` when GH Archive has no stars at all for the
  repo on a busy day from before June 2026. The repo may have had another
  name then. Follow [step 3](#3-follow-up-on-a-rename) before you explain
  the result.
- `repository` and `former_names`: the repo's current name, and the
  earlier names that GitHub forwards to it and that the check also looked
  up in GH Archive. When you checked an old name, `repository` is the new
  one and `former_names` holds the name you typed. Tell the user that the
  repo was renamed.
- `findings`: the evidence behind the verdict.
- `limits`: what the check could not see. Always pass on the ones that
  matter for this repo.
- `stars`, `forks`, `watchers` and `issues_and_prs_90_days`: the star
  count, and how much the repo is used besides its stars. They are context
  and never change the risk. `issues_and_prs_90_days` is `null` when the
  issues were not counted, for example because GitHub's search did not
  answer. Then tell the user that it was not counted. Do not say there
  were none.
- `days`: up to five of the busiest days. Each entry has its date in `day`,
  GitHub's calendar day written as midnight UTC, such as
  `2021-06-09T00:00:00Z`. Its star count is in `stars`.
- `days[].accounts`, only where `days[].judged` is `true`: then `one_off`
  out of `checked` is the share of accounts with no other public stars
  recorded in GH Archive within 30 days. On a day where `judged` is
  `false`, never report a share. Every day has both fields. When
  `accounts` is `null`, the accounts behind that day were not checked.
  Otherwise the day has a `problem` (GH Archive had too little data, and
  `checked` and `one_off` are 0) or is `provisional` (too recent to
  count). `accounts.window_recorded` says how well GH Archive recorded the
  weeks around the day, compared with the day's own week. `1` means as
  well, and less means more thinly. StarTrust already uses it to set
  `judged`, so you never need to weigh it yourself.
- StarTrust checks at most 1,000 accounts per day. When `checked` is
  smaller than `archived`, the share comes from a sample of 1,000 of the
  day's accounts. Say so when you report it. `archived` can be higher than
  `github_stars`, because GitHub drops the stars of deleted accounts and
  stars that were taken back, while GH Archive keeps them.

Do not compute your own verdict from the other fields, such as `days` or
`times_normal`. The verdict already compares them with what is known about
organic repos.

[JSON output](docs/json.md) describes every field.

If the user wants to see the chart, run the same command without `--json`
and show the output in a code block. For the full evidence, add
`--details`. To match busy days with Hacker News posts and releases, add
`--explain`.

## 3. Follow up on a rename

GH Archive files stars under the name a repo had at the time. When
`rename_suspected` is `true`, one of the `limits` gives the command to run,
for example:

```sh
startrust check withastro/astro --former-name OLD-OWNER/OLD-NAME
```

In `compare`, write the repo and its old name together, such as
`--former-name withastro/astro=snowpackjs/astro`.

1. Find the old name. Ask the user, or look for old links to the repo, for
   example in its changelog or release notes. GitHub does not list former
   names, so StarTrust cannot find them itself. Keep the capital letters of
   the old links, because GH Archive matches names exactly.
2. Run the command from the limit with `--json` and the old name in place
   of `OLD-OWNER/OLD-NAME`. StarTrust looks up every name in one check, so
   the new result covers the whole history. Use it instead of the first
   one, and tell the user that the check included the old name.
3. A correct name runs the whole check again. It uses as many GitHub
   requests as the first check (`sources.github_requests`), plus one for
   each former name. Look at `sources.github_rate_limit.remaining` before
   you run it.
4. A wrong name costs two requests. StarTrust asks GitHub whether the name
   leads to the repo, and it stops with exit code 1 when it does not. Then
   try another name or go on without one.
5. If neither you nor the user knows the old name, the check stays
   partial. StarTrust has no other way to check those days. Explain the
   first result, and give the risk with its coverage, such as "low,
   accounts checked on 1 of 3 busy days". Say that the accounts behind the
   other busy days were not checked, probably because the repo had another
   name then. Never present it as a full check. If you have a likely guess,
   such as the same name under an older organization, you can try it.
   GitHub only forwards names the repo had, so a wrong guess costs two
   requests and changes nothing.

If `rename_suspected` is still `true` with a former name, the repo may have
had yet another name. The limit then shows the command with every name so
far.

## 4. Explain the result

Tell the user what was found, in plain words, and quote the verdict. Then
explain the risk level:

- `low`: nothing unusual in what could be checked. If `days_checked` is
  smaller than `days_to_check`, this is a weaker result than a fully
  checked low. Say so, and say which days were not checked (the verdict and
  `limits` say why).
- `medium`: the timing looks odd, or a busy day had more one-off accounts
  than usual. Viral repos with a wide audience can look like this. This is
  a reason to look closer. It is not a finding of fraud.
- `high`: on a busy day, half or more of at least 100 checked accounts had
  no other public stars recorded in GH Archive, or 30% or more on a day
  with unusual timing. Bought or traded stars often look like this, but
  the check cannot prove it.
- `unknown`: StarTrust could not judge the repo. Usually the repo is new
  (younger than 240 days) or got most of its stars at its launch, and GH
  Archive had too few records to check the accounts. In 2026 this is
  common for new repos. It is not a pass and not a warning. Say that the
  stars could not be judged and quote the verdict. Then give the user the
  activity numbers from the JSON (`stars`, `forks` and
  `issues_and_prs_90_days`), so they can judge the repo themselves. As a
  reference, every organic repo in StarTrust's comparison had at least 10
  forks per 1,000 stars. But most repos flagged for bought stars also had
  normal numbers. Normal numbers do not mean the stars are real, so never
  tell the user they are. Few forks alone do not mean that the stars are
  fake. Tools such as `composer thanks` star library repos automatically,
  and those repos get almost no forks.

StarTrust reports patterns. It cannot see who sent the stars or why, and
anyone can send stars to any repo. Never say that the stars were bought.
Never say that the maintainers did anything, and never name a person as
responsible. If the verdict is medium, suggest `--explain` before drawing
any conclusion.

## 5. Add it to CI (only when asked)

Do this only when the user asks. Add a file under `.github/workflows/`
that uses the action, for example:

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
          repository: owner/repo
```

Use a released version tag after `@`. Add `fail-on: high` only if the user
wants the job to fail. It never fails on an unknown risk; the action adds a
warning to the run instead. [CI and badge](docs/ci-and-badge.md) has all the
inputs and the badge setup. Show the user the file before you commit it.

## 6. Check the user's own repo with --deep (only when asked)

`startrust check owner/repo --deep` lists who starred the repo. It needs a
token with push access to that repo and one extra request per 100 stars.
Use it only when the user maintains the repo and asks for it. If it stops
early, running the same command again within a day continues where it
stopped.

## Exit codes

- 0: the check ran. Read `risk` for the result.
- 1: wrong use, such as a bad repo name or flag. A `--former-name` that
  GitHub does not forward to the repo also gives 1. The message says why.
- 5: GitHub has no public repo by that name.
- 6: GitHub's rate limit is used up.
- 7: `--fail-on` matched. It never matches an unknown risk, so check `risk`
  yourself when that matters.
- 130: the check was interrupted. Any JSON printed before is a partial
  result.

The [troubleshooting guide](docs/troubleshooting.md#exit-codes) lists the
other codes.
