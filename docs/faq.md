# FAQ

## Why do people buy GitHub stars?

Stars are a quick signal of popularity. Investors and developers look at
them, and a repo that gets many stars quickly is more likely to show up on
GitHub's trending page. Researchers have found millions of suspected fake
stars, sold in packages or traded between accounts.

## Does StarTrust need my GitHub account?

No. You can check any public repo without a token. A token only raises
GitHub's limit from 60 to 5,000 requests an hour. StarTrust looks in
`GITHUB_TOKEN` first and then in `GH_TOKEN`. If neither is set, it asks the
GitHub CLI (`gh auth token`). It uses the token only to read from GitHub,
and it never prints or stores it.

The last lines of the report say which token the check used, such as "Used
your GitHub CLI login". They also say how many requests GitHub had left
after the check. In the JSON, `sources.token_source` and
`sources.github_rate_limit` say the same.
Add `--no-token` to check without a token, even when one is set.

## Does StarTrust have a config file?

No. You set everything with flags, and the token comes from the places
above. `--config` shows up in the help because every command accepts it,
but StarTrust ignores it. `--dry-run` is also on every command, but it only
does something for `cache prune`. The other commands only read data, so
there is nothing to preview. Saved checks go to the [cache](#where-is-the-cache).

## What does StarTrust send where?

StarTrust asks GitHub for the repo summary and its daily star history. It
also sends one search that counts the issues and pull requests opened in
the last 90 days. With `--explain` it adds a request for the repo's
releases, and with `--deep` it asks for the stargazer list of your own
repo. For each `--former-name`, it asks GitHub whether that name leads to
the repo.

For the account check, StarTrust sends the repo name and the dates of its
busiest days to ClickHouse's public endpoint, one query per day. A renamed
repo's former names go along. Use
`--no-archive` to leave this out.

Only with `--explain`, StarTrust sends the repo's GitHub URL and its
homepage to the Hacker News search that Algolia runs.

There is no telemetry. Results stay in your cache directory.

## How long does a check take?

Most checks take 5 to 15 seconds. About half of that goes to GitHub's
history, and half to the account check. A repo that is many years old
needs more history requests. `--deep` adds one request per 100 stargazers.

## What if the repo was renamed?

GH Archive files stars under the name a repo had at the time. Add the old
name with `--former-name`, and StarTrust looks up both names in the same
check. Checking the old name also works, because GitHub forwards it to the
repo. When a busy day suggests a rename, the Limits part of the report says
what to run. [Troubleshooting](troubleshooting.md#the-repo-was-renamed) has
the details.

## Why does the chart start before the first star?

The chart starts at the day the repo was created. Repos often get no stars
for a while before anyone notices them, and the chart shows that quiet
start.

## Why is a launch never "unusual"?

A new project goes from no stars to many in a day when it gets noticed.
That jump looks the same whether the stars are real or bought, so timing
can't judge it. StarTrust still checks the accounts behind a launch day,
and that check can raise the risk.

## Why does a new repo say "Too new to judge"?

Timing can't judge its launch, and a repo needs about 240 days of history
before StarTrust can find a burst in it. That leaves the account check. But
since mid-2025, GH Archive has recorded too few stars to check the accounts
behind most new repos. So StarTrust says it could not judge the repo, with
risk "unknown", instead of calling it fine.

The verdict then names the repo's forks and its issues and pull requests,
and the "Activity" part of the report puts them next to its stars. Those
numbers help you weigh a young repo yourself. If GH Archive records more of
the stars later, run the check again. If you maintain the repo, `--deep`
shows the profiles of your stargazers.

## Can forks or issues tell whether stars are bought?

Not on their own. StarTrust compared them for 15 repos flagged for bought
stars and 182 organic repos. Most flagged repos had numbers in the same
range as organic ones. Issues also vary a lot with the kind of repo. Model
releases and collections of agent skills get few of them.

No organic repo in that comparison had fewer than 10 forks per 1,000
stars, but two flagged repos did. So StarTrust tried a rule that raised the
risk of repos with fewer than 5. It dropped the rule again, because it
caught only those two flagged repos and also caught honest packages. Tools
such as `composer thanks` star library repos automatically, and those repos
get almost no forks (see [limits](limits.md#forks-and-issues-cannot-judge-a-repo)).
Now forks, watchers and issues are context, and they never change the
risk. [How it works](how-it-works.md#forks-watchers-and-issues) has the
full comparison.

## A repo I trust got "1 unusual spike". Is it in trouble?

Not necessarily. Medium risk means the timing looked odd, and nothing
could confirm or clear it. Projects that go viral long after they were
created often get an unusual spike. Run `startrust check owner/repo
--explain --details`. A Hacker News post or a release right before the
spike usually explains it. When GH Archive has data for that day, the
account check gives a clearer answer.

## A popular repo got "more one-off accounts". Why?

Some repos reach people who rarely star anything on GitHub, such as awesome
lists and repos about a news story or a cause. Many of those visitors have
no other public stars recorded around that day. In the calibration set,
viral repos like that reached up to 38% one-off accounts on their busiest
days. Developer tools stayed at 21% or less. So this share alone only makes
the risk medium.

## Does "unusual star pattern" mean the stars were bought?

No. It means that on at least one of the busiest days, many of the
accounts that starred had no other public stars recorded in GH Archive.
Bought or traded stars often look like that, but StarTrust cannot see who
gave the stars or why. Treat it as a reason to look closer before you trust
the star count.

## Can a maintainer fool StarTrust?

Probably, with effort. If the stars come in evenly over months, from
accounts that also star other repos, they would not stand out. StarTrust
notices sudden spikes and bursts between quiet periods, and it looks for
one-off accounts. [Limits](limits.md) lists what it misses.

## Can someone make my repo look bad?

Someone can buy stars for your repo, and StarTrust would report the
pattern. So its reports describe what the stars look like and never accuse
the maintainers. If you think this happened to you, `--deep` shows the
profiles of the accounts behind each busy day. That helps when you report
them to GitHub.

## Why not use the stargazer list for every repo?

Since July 2026, GitHub only shows who starred a repo to people with push
access to it. For other repos it answers "not found", or returns an empty
list with no error. StarTrust uses the list only with `--deep` on your own
repos, and notices when GitHub hides it.

## Where is the cache?

It depends on your system:

- Linux: `~/.cache/startrust`
- macOS: `~/Library/Caches/startrust`
- Windows: `%LocalAppData%\startrust`

Each finished check is one JSON file. `startrust cache list` shows them,
and `startrust cache prune --dry-run` shows what a cleanup would delete.
`--cache-dir` points StarTrust at another folder.
