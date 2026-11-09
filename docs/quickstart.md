# Quickstart

This page takes you from install to your first check, and shows how to read
the result.

## Install

On Linux and macOS, or on Windows in Git Bash:

```sh
curl -fsSL https://raw.githubusercontent.com/vincentsch/startrust/main/install.sh | sh
```

The installer downloads the release binary for your system and checks its
SHA-256 checksum. Then it puts the binary in `~/.local/bin`. Set
`INSTALL_DIR` to put it somewhere else, or `STARTRUST_VERSION=v0.1.0` to
install a given release. If the installer prints a PATH line, run it.

On Windows without Git Bash, download `startrust-windows-amd64.exe` and
`checksums.txt` from the latest release. Compare the output of
`Get-FileHash -Algorithm SHA256 startrust-windows-amd64.exe` with the line in
`checksums.txt` before you run it.

With Go 1.27 or newer you can build from source instead:

```sh
go install github.com/vincentsch/startrust/cmd/startrust@latest
```

Check that it works:

```sh
startrust --version
```

## Check a repo

```sh
startrust check owner/repo
```

You can also paste a GitHub URL, such as
`startrust check https://github.com/junegunn/fzf`. An old name of a renamed
repo works too, because GitHub forwards it. The report then shows the
current name, and the line under it says which name GitHub forwarded.

A check needs no account. Without a token, GitHub allows 60 requests an hour
from your IP address. A check needs one request for each 30 weeks of the
repo's age, plus two more for the repo and its recent issues. So a
ten-year-old repo needs about 20. StarTrust prints the estimate before it
starts. To get 5,000 requests an hour, log in with the GitHub CLI (`gh auth
login`) or set `GITHUB_TOKEN`. StarTrust uses the token only to read from
GitHub, and it never prints it. The last lines of the report say which
token the check used and how many requests were left this hour.
`--no-token` checks without a token.

## Read the result

The verdict comes right after the header. It describes what the check saw.
It never says who starred or why. Below it is the risk level, and on how
many of the busy days StarTrust could check the accounts, such as "Risk:
low, accounts checked on 3 of 3 busy days". When fewer days were checked,
the risk rests partly on the timing of the stars, and the verdict says
which days are missing. The verdict starts with one of these:

- "No unusual pattern" means that StarTrust found no unusual spike and no
  burst, or that the accounts behind them looked like most stargazers.
  This is risk "low". When the accounts behind some busy days could not be
  checked, the verdict says so, for example "No unusual pattern in the star
  timing. The accounts behind the launch day could not be checked."
- "One unusual spike, see below." (or "2 unusual spikes", or "Most stars
  arrived in one burst") means that the timing looks odd and no account
  check could clear it. This is risk "medium".
- "More one-off accounts than usual" means that on a busy day, 30% or more
  of the accounts had no other public stars recorded in GH Archive within
  30 days, but not enough for risk "high" (see [risk
  levels](how-it-works.md#risk-levels)). Viral repos with a wide audience
  can also look like this. This is also risk "medium".
- "Unusual star pattern" means that on a busy day, half or more of at least
  100 checked accounts had no other public stars recorded in GH Archive
  within 30 days. A share of 30% or more also counts when that day was part
  of an unusual spike or a burst. Bought or traded stars often look like
  this. This is risk "high".
- "Too new to judge" or "Not enough data to judge" means that StarTrust
  could not judge the repo. Usually the repo is younger than 240 days or got
  at least half of its stars at its launch, and GH Archive could not check
  the accounts. Then the verdict also names the repo's forks and its issues
  and pull requests, so you can weigh it yourself. "Not enough data to
  judge" also shows when StarTrust could read only part of the history and
  found nothing in that part. In all these cases the risk is "unknown",
  which is not a pass. A repo without any stars gets "No stars yet", also
  with risk "unknown".

"Activity" comes next. It lists the repo's forks and watchers, and the
issues and pull requests people opened in the last 90 days. A star costs
one click, while these usually come from people who use the project. But
most repos flagged for bought stars also had normal numbers here, so normal
numbers do not clear a repo. Few forks also do not flag a repo. Tools such
as `composer thanks` star library repos automatically, and those repos get
almost no forks. So these numbers never change the risk. Every organic repo
in StarTrust's comparison had at least 10 forks per 1,000 stars, and the
report shows that as a reference. See [forks, watchers and
issues](how-it-works.md#forks-watchers-and-issues).

Each bar of the chart covers a day for a young repo, and a longer period
for an older one. The numbers under the chart point to the busy days in the
list below it. Two busy days in the same bar share a label, such as "1,5".

The list shows the busiest days, at least a week apart. Each day shows its
stars and how that compares with a normal day in the four weeks before
("9x normal"). When a normal day had no stars at all, there is nothing to
compare with, and the list says "quiet before". Some days get a label:

- "launch" means that the repo had fewer than 100 stars before.
- "spike" means that the day is part of a spike. A spike needs at least 30
  stars and 10 times a normal day.
- "unusual spike" means that the spike's biggest day had at least 300 times
  a normal day, and the repo had 100 stars or more before it.
- "part of 1" means that the day belongs to the same spike as day 1 in the
  list.

The last column shows the share of one-off accounts from GH Archive. It
says "too few in GH Archive" when the archive has too little data for that
day or the weeks around it. It says "so far (provisional)" when the 30 days
after the day are not over yet, and such a share does not count until
then. A share from fewer than 100 accounts shows its counts, such as "9 of
30 one-off". StarTrust checks at most 1,000 accounts per day. When GH
Archive has more, the share comes from a sample of 1,000, and the list
marks it with "(sample)".

"What stands out" explains the evidence behind the verdict. "Limits" says
what the check could not see. [How the verdict works](how-it-works.md) has
every rule.

## See the evidence

```sh
startrust check owner/repo --details
```

`--details` adds the numbers behind the verdict. You get every spike with
its dates, and what GH Archive found for each day.

```sh
startrust check owner/repo --explain
```

`--explain` looks for Hacker News posts that link to the repo, and for
GitHub releases. It shows the ones from each busy day or the two days
before it, and says so when it finds none. If a busy day comes right after
a post on the Hacker News front page, the post usually explains the stars.

## Compare repos

```sh
startrust compare junegunn/fzf charmbracelet/glow jesseduffield/lazygit
```

`compare` checks two to eight repos and puts them in one table, with their
risk and their biggest day. The "Accounts checked" column next to the risk
says on how many busy days the accounts were checked, such as "1 of 3
days". Below the table it draws their star histories on one timeline and
prints each verdict.

## Save and share a report

```sh
startrust check owner/repo --markdown --out stars.md
```

The Markdown report has the same content as the terminal view, with the
chart in a code block. It renders on GitHub, so you can paste it into an
issue or a pull request.

Every check is also saved in your cache, so you can show it again later
without asking GitHub:

```sh
startrust report owner/repo
startrust report owner/repo --markdown
startrust cache list
```

## Use it from a script or an agent

```sh
startrust check owner/repo --json
```

`--json` prints a compact summary with the risk and the verdict. It also
lists the busy days with their account checks. Add `--details` to include
the full daily history. [JSON output](json.md) describes every field.

## Check your own repo more closely

```sh
startrust check your/repo --deep
```

If you have push access to the repo, GitHub lets you see who starred it.
`--deep` lists your stargazers, 100 per request. For each busy day, it
counts the accounts that were new or empty. New means the account was
created at most 30 days before it starred. Empty means it has no followers
and no public repositories. A small repo has no busy day with enough
stargazers for that, so it gets the counts for all of them. This helps when
you want to know whether someone sent stars to your repo. A repo with 5,000
stars needs 50 extra requests. If you stop the run with Ctrl-C, the
accounts listed so far are saved. Run the same command again within a day,
and it continues from there.

## Next steps

- [CI and badge](ci-and-badge.md): check a repo in GitHub Actions and show
  a badge.
- [Limits](limits.md): what StarTrust cannot tell you.
- [FAQ](faq.md) and [troubleshooting](troubleshooting.md).
