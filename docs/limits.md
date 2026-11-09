# Limits

StarTrust reports patterns in public data. This page lists what that data
cannot show, so you can weigh a verdict before you act on it.

## A pattern is not proof of who did it

Anyone can star any repo, and anyone can pay for stars to go to any repo.
A star campaign can come from the maintainers. It can also come from
someone else, even from someone who wants the project to look bad.
StarTrust says what the stars look like. It cannot say who arranged them,
and its reports never name a person.

Some campaigns happen in the open. Crypto projects sometimes reward people
for starring their repo during an airdrop, which is a giveaway of free
tokens. Those accounts belong to real people, but they starred for the
reward. StarTrust cannot tell them apart from other one-off accounts. The
verdict describes the pattern ("unusual star pattern") and never says that
stars were bought.

## Wide audiences look a bit like campaigns

Some honest repos reach people who rarely star anything, such as awesome
lists and repos about a cause or the news. In the [calibration
set](how-it-works.md#calibration), up to 38% of the accounts on their
busiest days had no other public stars recorded in GH Archive. Developer
tools stayed at 21% or less. So StarTrust only calls a day high on its own
at 50% or more. A day between 30% and 50% only counts as high when its
timing is also unusual.

## Timing alone is often unclear

Launches make spikes, and so do Hacker News posts and big releases. A
bought spike can look the same. In the calibration set, five honest repos
had an unusual spike, mostly when they went viral long after they were
created. When the account check has no data for a day, the best StarTrust
can say is that a spike is unusual. `--explain` helps by showing Hacker
News posts and releases near busy days.

## Forks and issues cannot judge a repo

Every report lists the repo's forks and watchers, and its issues and pull
requests from the last 90 days. Most repos flagged for bought stars had
normal numbers there, and one of them had more forks than stars. So normal
numbers say nothing about whether the stars are real.

Very few forks also say nothing. Tools such as `composer thanks` star
every package that a project depends on. So a small package that many
projects install, such as one of Symfony's polyfill packages, can get
thousands of stars and almost no forks. StarTrust shows these numbers as
context, and they never change the risk.

## GitHub only counts stars that still exist

GitHub's daily history only includes stars from accounts that still
exist. When GitHub deletes fake accounts, their stars disappear from the
history. A campaign that GitHub already cleaned up can look smaller than it
was, or not show at all.

## GH Archive has gaps

GH Archive records GitHub's public events from outside GitHub, and it does
not catch all of them. Since June 2025 it has missed a growing share of all
stars. To measure how much, StarTrust compared the stars GH Archive
recorded for 28 organic repos of the calibration set with GitHub's own
daily counts for the same repos, month by month. This was measured on
2026-10-26, through ClickHouse's copy of GH Archive.

| Months | Stars GH Archive recorded, as a share of GitHub's count |
| --- | --- |
| January 2023 to April 2025 | 97% to 109% |
| May 2025 | 92% |
| June to September 2025 | 58% to 64% |
| October 2025 to January 2026 | 37% to 43% |
| February 2026 | 33% |
| March and April 2026 | 17% to 19% |
| May 2026 | 11% |
| June to August 2026 | 1% to 3% |
| September 2026 | 10% |
| October 2026 | 2% |

Shares over 100% are normal. GitHub drops the stars of accounts that were
deleted later, but GH Archive still has them. The short rise in September
2026 came from a few days around the start of the month.

The Limits part of every report says whether this affects the check. It
names the busy days from June 2025 on whose accounts were checked, or it
says that all of them are older.

StarTrust judges a day only when GH Archive has at least 40% of its stars,
and the weeks around it are recorded about as well (see [the account
check](how-it-works.md#the-account-check)). So most busy days from late
2025 on show "too few in GH Archive". For a repo created in 2026, that
usually means no account check at all. Timing also cannot judge its
launch, so the verdict is "Too new to judge" or "Not enough data to judge",
with risk unknown. This is not a pass.

GH Archive files events under the name a repo had at the time. GitHub
forwards the old names of a renamed repo to it, but it does not list them,
so StarTrust cannot find them on its own. It looks up the current name and
the name you typed. It also looks up every name you add with
`--former-name`. If the repo had another name on a busy day, StarTrust misses the stars given under that
name. When GH Archive has no stars at all for a busy day from before June
2026, the Limits part of the report gives the command to run with
`--former-name`, and the JSON has `"rename_suspected": true`. On later days
GH Archive misses almost every star anyway, so a day without any record
says nothing about the name. [Troubleshooting](troubleshooting.md#the-repo-was-renamed)
shows how to find a former name.

StarTrust reads GH Archive through ClickHouse's free public endpoint. It is
a demo service with an hourly limit per user, and it can be slow or down.
When that happens, the check still runs and says why the account check is
missing.

## Accounts that star a lot look normal

The account check looks for accounts with no other public stars around
the busy day. Some star services use accounts that star hundreds of repos
to look active. Those accounts pass the check. The StarScout research
project found such groups by looking for accounts that star the same repos
at the same time. StarTrust does not do that yet. In the calibration set,
three of the four repos that StarScout flagged for this came out low.

## Small samples and recent days

StarTrust only judges a day when GH Archive has at least 30 of its
stargazers. Repos with a few hundred stars often have no day that
qualifies. For them, StarTrust can only judge the timing, and the verdict
says so. A day needs at least 100 sampled accounts before it can make the
risk high. A share from fewer than 100 accounts shows its counts, such as
"9 of 30 one-off".

A day from the last month is provisional. The 30 days after it are not
over, so its accounts may still star other repos. StarTrust shows its share
but leaves it out of the risk, and the Limits part of the report says when
to check again.

The thresholds come from 46 repos (see [how the verdict
works](how-it-works.md#calibration)). That is enough to separate the clear
cases in that set. It is not enough to promise how StarTrust does on every
repo, so the badge says "experimental".

## Your own repo with --deep

`--deep` shows how many stargazers on each busy day had new or empty
accounts. New GitHub users do star projects they hear about, so these
counts are shown as evidence only. They do not change the risk.
