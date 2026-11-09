# How the verdict works

StarTrust looks at two things. The first is when a repo's stars arrived.
The second is whether the accounts behind its busiest days also star other
repos. The verdict says what StarTrust saw. It never says who gave the
stars or why.

This page has every rule behind the verdict. The numbers in the rules come
from 46 repos that StarTrust was tested on, the calibration set (see
[Calibration](#calibration)). Of these, 31 are organic repos, which means
well-known projects with a public story behind their stars. The other 15
were flagged for fake stars by research or press reports.

## Where the data comes from

The first source is GitHub's daily star history. For every public repo,
GitHub reports how many stars arrived on each day since the repo was
created. It only counts stars that still exist, so stars from deleted
accounts are gone. The days follow US Pacific time. One request returns 30
weeks, so a repo that is 13 years old needs 23 requests. This works
without a token.

The second source is [GH Archive](https://www.gharchive.org), an
independent project that collects GitHub's public events and publishes
them. For each star it records, it also has the account that gave it.
StarTrust reads the free public copy that the database company ClickHouse
runs at [play.clickhouse.com](https://play.clickhouse.com). These queries
go to ClickHouse and need no account, and they don't count against your
GitHub limit. Since July 2026 this is the only public way to see who starred a
repo you don't maintain.

For the repo's activity, StarTrust reads the forks and watchers from
GitHub's summary of the repo. One search request counts the issues and pull
requests opened in the last 90 days. See [forks, watchers and
issues](#forks-watchers-and-issues).

Two more sources are optional. With `--deep`, maintainers get the list of
who starred their own repo from GitHub. With `--explain`, StarTrust looks
for Hacker News posts and GitHub releases near the busy days. It searches
Hacker News through Algolia's free search API.

## Spikes

StarTrust compares each day with a normal day. A normal day is the median
of the 28 days before, which is the middle value when you sort those days
by their stars. A normal day of less than one star counts as one, so a
quiet repo's first day with 50 stars is 50 times normal. The report shows
no such ratio when a normal day had no stars at all, because it would only
repeat the star count. It says "quiet before" instead, and the JSON has
`null`.

A day is a **spike** when it has at least 30 stars and at least 10 times a
normal day. Spike days at most 3 days apart count as one spike. A spike
also goes on while the next day has at least 30 stars and at least half the
stars of the spike's biggest day. This way a launch week counts as one
spike. The spike's biggest day is compared with the 28 days before that
day.

A spike is a **launch** when the repo had fewer than 100 stars before it.
Every launch jumps from nothing, and it looks the same with real or bought
stars. So timing alone never calls a launch unusual.

A spike is **unusual** when it is not a launch and its biggest day had at
least 300 times a normal day. Honest repos also get unusual spikes. In the
calibration set, five organic repos had one, most of them when they went
viral long after they were created. youtube-dl got 2,919 times a normal day
when it came back after its 2020 takedown. The unusual spikes of the
flagged repos were in the same range, from a few hundred to a few thousand
times a normal day. So an unusual spike alone only makes the risk medium,
and the account check can clear it.

Plenty of organic repos have smaller spikes. fzf has five days over 10
times normal in 13 years. StarTrust lists these spikes, but only unusual
spikes count against a repo.

## Bursts

StarTrust finds the 90 days with the most stars. It reports a **burst**
when all of these hold:

- Those 90 days hold at least half of all the repo's stars, and at least
  1,000 stars.
- The repo existed for at least 60 days before them. In those days (up to
  180 of them), it got less than 5% of the burst's daily rate.
- In the last 90 days, it again got less than 5% of that rate.

The burst must have ended at least 90 days ago, so StarTrust can measure
the quiet period after it. Paid campaigns often bring thousands of stars
between two quiet periods, and popular projects rarely stop that suddenly.
A burst needs 60 days of history before it, so a project that goes viral
in its first two months never counts as a burst. Small repos often get most
of their stars from one post somewhere, so a burst needs at least 1,000
stars.

All this means that a repo needs about 240 days of history before
StarTrust can find a burst in it.

In the calibration set, seven flagged repos had a burst and no organic repo
did. Like an unusual spike, a burst alone only makes the risk medium.

## The account check

StarTrust checks the accounts behind up to 3 of the busiest days. Each of
these days has at least 50 stars, and they are at least 7 days apart.
StarTrust also checks the biggest day of each unusual spike.

For each day, it asks GH Archive who starred the repo that day, on
GitHub's Pacific calendar. It takes a random sample of up to 1,000 of those
accounts, the same sample each time. The report marks a share from such a
sample with "(sample)". For each account in the sample, it
counts the public stars the account gave in the 30 days before and after.
An account is **one-off** when GH Archive recorded no other public star
from it in those two months.

People who star a repo because they found it usually star other repos
around the same time. In the calibration set, the median account that
starred a developer tool gave 10 to 44 stars in those two months. Accounts
used in star campaigns often exist to star one or a few repos.

StarTrust only judges a day when GH Archive has at least 30 of its
stargazers and at least 40% of the stars GitHub counts for that day. GH
Archive can also have more stargazers than GitHub counts. GitHub drops a
star when the account is deleted or the star is taken back, but GH Archive
keeps its record. Such a day counts as fully recorded. The
two months around the day also matter. When GH Archive misses stars there,
it also misses the other stars of these accounts, and they look one-off
when they are not. So the same query counts all stars that GH Archive
recorded across GitHub in those 61 days, and compares that with the week
of the day. When the 61 days are recorded more thinly than the week,
StarTrust takes that into account. Say GH Archive has 60% of a day's
stars, but in the 61 days around it, it recorded only half as many stars a
day as in that week. Then the day counts as having 30% (60% times one
half). That is under 40%, so the day is not judged. Busy days that cannot
be judged show "too few in GH Archive".

Since June 2025, GH Archive has missed a growing share of all stars, and
since June 2026 almost all of them. [Limits](limits.md#gh-archive-has-gaps)
has the numbers by month. Most busy days from late 2025 on cannot be
judged.

A day also needs its 30 days after to be over, plus two days for GH Archive
to catch up. Before that, an account that will star another repo next week
still looks one-off. StarTrust shows the share for such a day as
"provisional" and leaves it out of the risk until then.

A check sends up to 5 queries to ClickHouse, usually 3. Each query reads
about half a billion rows, and the free endpoint allows 300 billion rows an
hour. Answers for days older than a month never change, so StarTrust saves
them in its cache.

## Risk levels

| Risk | When |
| --- | --- |
| high | On a checked day, 50% or more of at least 100 sampled accounts were one-off. Or 30% or more of at least 100, on a day that was part of an unusual spike or a burst. |
| medium | On a checked day, 30% or more of the accounts were one-off. Or there is an unusual spike or a burst that no account check could clear. |
| low | None of the above. The verdict says when the accounts behind the busy days could not be checked. |
| unknown | StarTrust could not judge the repo. Usually no account check counted, and the timing cannot tell (see below). A repo without stars is also unknown. So is a repo whose history StarTrust could only read in part, unless that part already shows something. |

### Why are there two thresholds?

Some honest repos reach people who rarely star anything, such as awesome
lists (curated lists of links) and repos about a cause or the news. On
their busiest days, up to 38% of the accounts were one-off. Developer tools
stayed at 21% or less. So a share between 30% and 50% only counts as high
when the timing of that day is also unusual. Then two separate signals
agree.

A small sample can be off by chance. With 30 accounts, chance alone can
move the share by about 15 percentage points. So a day needs at least 100
sampled accounts to make the risk high. A share from fewer than 100
accounts shows its counts, such as "9 of 30 one-off".

### When does StarTrust say it cannot judge a repo?

Timing never judges a launch, because a launch looks the same with real or
bought stars. And StarTrust can only find a burst in a repo with
about 240 days of history. So when no account check counted, StarTrust
calls the risk unknown in two cases:

- The repo is younger than 240 days. The verdict starts with "Too new to
  judge".
- At least half of its stars came with its launch. The verdict starts with
  "Not enough data to judge".

In 2026 this covers most new repos, because GH Archive has too few records
to check their accounts. Saying "no unusual pattern" there would sound like
a pass for a repo nobody could judge.

`--fail-on` never matches an unknown risk. StarTrust says so on stderr,
and the GitHub Action adds a warning to the run.

### What the verdict says

The verdict says which rule decided. These are its forms, with made-up
numbers:

- high: "Unusual star pattern. On 2 of the busiest days, 55% to 80% of the
  accounts that starred had no other public stars recorded in GH Archive
  within 30 days. Bought or traded stars often look like this, so look
  closer before you trust the star count."
- medium: "More one-off accounts than usual. On 2019-04-08, 38% of the
  accounts that starred had no other public stars recorded in GH Archive
  within 30 days. See below." Or, by timing alone: "One unusual spike, see
  below."
- low, with the accounts checked: "No unusual pattern. The stars arrived
  steadily, and the biggest day had 180 stars, 7x a normal day." When the
  accounts cleared an unusual spike: "No unusual pattern. The accounts
  behind the unusual spike on 2025-09-09 look like most stargazers (17%
  one-off)."
- low, when some accounts could not be checked: "No unusual pattern in the
  star timing. The accounts behind the launch day could not be checked."
- unknown: "Too new to judge. The repo is 21 days old, and timing alone
  cannot tell a launch from a star campaign. GH Archive has too few records
  of the busiest days to check the accounts. So far it has 45 forks and 40
  issues and pull requests, see below."

Below the verdict, the terminal shows the risk level, such as "Risk: low".
In the JSON, the risk is in `risk`, and `headline` sums up the result in a
few words. The badge shows the headline. `--details` shows all the numbers
behind the verdict.

## Calibration

These are the results of `startrust check` on 2026-10-26. The "Risk"
column is the normal check. The "No account check" column is the same check
with `--no-archive`, so it rests on the timing of the stars. The
tests replay every row from saved data, so a change to a rule shows up as a
failing test.

The organic repos are well-known projects of different kinds, with a public
history that explains their popularity. The Kind column says what each one
is. The flagged repos come from press reports about fake stars and from the
published list of [StarScout](https://github.com/hehao98/StarScout), a
study of fake stars on GitHub (He et al., ICSE 2026). StarScout's authors
warn that single repos on their list may be false positives. They also say
that the list is meant for statistics about many repos, and that it should
not be used to judge a single repo. So this page does not name the flagged
repos, and it rounds their shares outward to whole tens, which makes each
range a bit wider.

| Repo | Kind | Timing | One-off accounts on the checked days | Risk | No account check |
| --- | --- | --- | --- | --- | --- |
| Repo A | flagged | unusual spike, burst | 40% to 90% | high | medium |
| Repo B | flagged | unusual spike, burst | 50% to 100% | high | medium |
| Repo C | flagged | unusual spike, burst | 60% to 90% | high | medium |
| Repo D | flagged | unusual spike, burst | 80% to 90% | high | medium |
| Repo E | flagged | burst | 90% to 100% | high | medium |
| Repo F | flagged | nothing | 60% to 100% | high | low |
| Repo G | flagged | burst | 40% to 60% | high | medium |
| Repo H | flagged | nothing | 60% to 80% | high | low |
| Repo I | flagged | launch | 70% to 80% | high | unknown |
| Repo J | flagged | burst | 30% to 70% | high | medium |
| Repo K | flagged, lockstep accounts | launch | 0% to 20% | low | unknown |
| Repo L | flagged, lockstep accounts | launch | 0% to 10% | low | unknown |
| Repo M | flagged, lockstep accounts | unusual spike | 0% to 70% | high | medium |
| Repo N | flagged, lockstep accounts | nothing | 0% | low | low |
| Repo O | flagged, smaller share | nothing | 20% to 50% | medium | low |
| junegunn/fzf | developer tool | nothing | 2%, 3%, 2% | low | low |
| charmbracelet/glow | developer tool | nothing | 3%, gap, 2% | low | low |
| openai/codex | launch | launch | 9%, too few, too few | low | low |
| astral-sh/uv | launch | launch | 4%, 10%, 5% | low | low |
| jesseduffield/lazygit | launch | launch | 3%, 4%, 5% | low | low |
| yoheinakajima/babyagi | viral launch | launch | 4%, 2%, 6% | low | low |
| vincentsch/explainroo | launch in 2026 | launch | too few | unknown | unknown |
| openai/whisper | launch | launch | 8%, 7%, 8% | low | low |
| twitter/the-algorithm | viral launch, news | unusual spike | 16%, 17%, 19% | low | medium |
| Significant-Gravitas/AutoGPT | viral launch | unusual spike | 13%, 20%, 7% | low | medium |
| deepseek-ai/DeepSeek-R1 | viral launch, news | nothing | 14%, 5%, 12% | low | low |
| xai-org/grok-1 | viral launch, news | launch | 21%, 18%, too few | low | unknown |
| hacksider/Deep-Live-Cam | viral long after creation | unusual spike | 13%, too few, 21% | low | medium |
| Stirling-Tools/Stirling-PDF | viral long after creation | nothing | 9%, too few, 10% | low | low |
| ytdl-org/youtube-dl | viral long after creation | unusual spike | 12%, 7%, too few | low | medium |
| sindresorhus/awesome | awesome list | nothing | 24%, 19%, 14% | low | low |
| vinta/awesome-python | awesome list | launch | 6%, 12%, 6% | low | low |
| public-apis/public-apis | list | nothing | too few, too few, 22% | low | low |
| EbookFoundation/free-programming-books | list | nothing | 36%, 10%, 16% | medium | low |
| nilbuild/developer-roadmap | learning roadmap | launch | 8%, 18%, 10% | low | low |
| jackfrued/Python-100-Days | course | nothing | 17%, 21%, 26% | low | low |
| 996icu/996.ICU | viral cause repo | launch | 29%, 38%, 34% | medium | low |
| kattgu7/Anti-996-License | cause repo, small | nothing | 2% | low | low |
| wuhan2020/wuhan2020 | viral cause repo | launch | 22%, 22% | low | unknown |
| CSSEGISandData/COVID-19 | data repo | nothing | 18%, 17%, 23% | low | low |
| owid/covid-19-data | data repo | nothing | 4%, 5%, 4% | low | low |
| nytimes/covid-19-data | data repo | launch | 24%, 25% | low | low |
| rxi/microui | small repo | nothing | too few, 7%, 3% | low | low |
| antonmedv/countdown | small repo | launch | 0% | low | low |
| nalgeon/sqlean | small repo | unusual spike | 4%, too few, 4% | low | medium |
| zserge/jsmn | small repo | nothing | no day with 50 stars | low | low |

What the table shows:

- No organic repo came out high. Two came out medium, free-programming-books
  and 996.ICU. Both have a very wide audience.
- In this calibration set, organic developer tools and launches had up to
  21% one-off accounts on a checked day. Repos with a wider audience, such
  as lists and repos about a cause, had up to 38%. Of the 25 checked days
  of repos A to J, 22 had 50% or more.
- StarScout flagged four repos (K to N) for lockstep accounts. These are
  groups of accounts that star the same repos at the same time. They star
  many repos, so the one-off check does not catch them, and three of the
  four came out low. Repo M came out high because of two later days, months
  after the days StarScout flagged.
- Without the account check, five organic repos and eight flagged ones
  came out medium, all by their timing. The account check cleared the five
  organic ones.
- Without the account check, six repos could not be judged, so their risk
  is unknown there. Most of their stars came with the launch, or the repo
  is younger than 240 days. Three of the six are flagged (I, K and L).
- "gap" marks a day on which GH Archive had enough of the day's stars, but
  missed too many stars in the weeks around it (glow, November 2025).

Besides the days in the table, the same query ran on 15 more busy days of
the organic repos. The highest share was again 38%, on 996.ICU. Still, 46
repos cannot promise how StarTrust does on every other repo, so the badge
says "experimental". If you find a repo where the verdict looks wrong,
please open an issue with the `--json --details` output.

Some repos were renamed since their busy days. GH Archive files events
under the name a repo had at the time, so these rows were also checked
under their old names. You do that with `--former-name`, for example
`startrust check Significant-Gravitas/AutoGPT --former-name Torantulino/Auto-GPT`.
Checking the old name itself also works.

The calibration data is in the repo. `internal/stars/testdata/` holds the
star histories and the GH Archive answers. The flagged repos are stored
under made-up names, with their dates moved and all their counts scaled
down by 5 to 15 percent, so they cannot be looked up. This does not change
any result in the table. `internal/check/calibration_test.go` replays the
table above on every test run.

## Forks, watchers and issues

Every report shows how many forks and watchers the repo has, and how many
issues and pull requests people opened in the last 90 days (for a repo
younger than that, all of them). Watchers are the people who get GitHub's
notifications for the repo. A star costs one click. The other numbers
usually come from people who use the project. So they can help you judge
a repo that StarTrust cannot judge by its stars, such as most repos
created in 2026.

They never change the risk, because they do not tell flagged repos from
organic ones. This is the comparison behind that decision.

StarTrust compared the 15 flagged repos of the calibration set with 132
organic repos. These are the 31 organic repos of the calibration set and
101 more of different sizes and kinds. Some of them are many years old,
and 40 are AI projects that well-known companies and people launched in
2025 and 2026, because those are the repos people check most right now.
For each repo, StarTrust took the counts from the GitHub API on 2026-10-26
and divided them by the repo's stars. In the table, each cell shows the
lowest to the highest value per 1,000 stars, with the median in brackets.

| Group | Repos | Forks per 1,000 stars | Watchers per 1,000 stars | Issues and PRs per 1,000 stars |
| --- | --- | --- | --- | --- |
| Developer tools and apps | 20 | 19 to 197 (48) | 1.9 to 24 (3.3) | 30 to 571 (106) |
| Libraries | 13 | 56 to 228 (109) | 3.2 to 28 (9.8) | 35 to 396 (97) |
| AI projects launched in 2025 | 14 | 86 to 193 (141) | 3.9 to 12 (7.0) | 16 to 674 (82) |
| AI projects launched in 2026 | 26 | 54 to 267 (98) | 2.3 to 22 (5.0) | 0.0 to 764 (9.9) |
| Older AI projects and model releases | 15 | 82 to 244 (146) | 0.1 to 25 (7.1) | 1.8 to 240 (19) |
| Lists and courses | 16 | 47 to 531 (117) | 0.5 to 32 (14) | 0.0 to 170 (7.3) |
| Cause, data and news repos | 7 | 47 to 629 (179) | 0.1 to 45 (20) | 7.2 to 458 (73) |
| Small repos | 20 | 21 to 196 (53) | 1.7 to 34 (9.6) | 9.3 to 207 (41) |
| A mirror (torvalds/linux) | 1 | 266 | 33 | 0.0 |
| All organic repos | 132 | 19 to 629 (99) | 0.1 to 45 (7.1) | 0.0 to 764 (43) |
| Flagged repos | 15 | under 10 to about 1,200 (about 60) | 0 to about 40 (about 5) | under 10 to about 900 (about 20) |

Today's numbers mix in everything that happened after a flagged repo's
campaign, which for most of them was years ago. A young repo checked today
is closer to a flagged repo right after its campaign. So StarTrust also
counted the forks and the issues and pull requests up to 30 days after
each repo's busiest day. It did that for the 46 calibration repos and
for 39 of the repos launched in 2025 and 2026.

What the numbers show:

- Most flagged repos look like organic ones. Their numbers per 1,000 stars
  fall inside the organic range, today and right after their busiest day.
- Issues and pull requests depend on the kind of repo. Model releases and
  collections of agent skills get read and starred, but get few bug
  reports. Thirty days after their busiest day, 11 of 70 organic repos had
  fewer than 5 issues and pull requests per 1,000 stars, and so did 5 of
  the 15 flagged repos.
- Watchers are low for both groups. Some organic repos had 0.1 per 1,000
  stars.
- No organic repo had fewer than 16 forks per 1,000 stars, today or 30
  days after its busiest day. To test this, StarTrust also checked 50
  organic repos picked to have few forks. Most of them are command-line
  tools and desktop apps that people install without ever forking them.
  The lowest had 18. Two flagged repos had under 4.
- Forks lag behind stars in the first days of a launch. One day after its
  busiest day, uv had 11 forks per 1,000 stars and explainroo 16. Two days
  later they had 28 and 29.

Watchers and issues do not separate the two groups at all. Forks looked
like they might, in one narrow case. So StarTrust tried a rule for them,
and then dropped it.

### Why the few-forks rule was dropped

The rule said a repo has **very few forks** when it has fewer than 5 forks
per 1,000 stars, at least 1,000 stars, and is older than 14 days. Such a
repo got at least risk medium. 5 is about half the lowest value any organic
repo had above (uv, one day after its busiest day). The JSON had a
`few_forks` field for the rule.

On the data above, the rule never fired on an organic repo. But it caught
only 2 of the 15 flagged repos (E and I). Then a last round of testing
found honest repos that it did catch. Tools such as `composer thanks` star
every package that a project depends on. So small packages that many
projects install, such as Symfony's polyfill packages, get thousands of
stars and almost no forks. symfony/polyfill-php72 has about 1 fork per
1,000 stars. A rule that catches 2 flagged repos and a whole kind of honest
repo should not raise the risk. So StarTrust removed the rule, and the
`few_forks` field with it.

Now forks are context, like watchers and issues. Every report shows them
next to one fact from the comparison. All 182 organic repos had at least
10 forks per 1,000 stars, even in the first days of a launch. The same note
says that few forks alone mean nothing. When StarTrust cannot judge a repo
by its stars, the verdict also names its forks and its issues and pull
requests.

This table shows the flagged repos, rounded so they cannot be looked up.
The last two columns are also per 1,000 stars, counted 30 days after the
busiest day.

| Repo | Forks per 1,000 stars | Watchers per 1,000 stars | Issues and PRs per 1,000 stars | After the busiest day: forks | After the busiest day: issues and PRs |
| --- | --- | --- | --- | --- | --- |
| Repo A | about 50 | about 20 | about 70 | about 70 | about 200 |
| Repo B | about 100 | about 5 | about 60 | about 80 | about 70 |
| Repo C | about 20 | about 10 | about 20 | about 30 | about 10 |
| Repo D | about 20 | under 5 | about 200 | about 20 | about 200 |
| Repo E | under 10 | under 5 | about 10 | under 10 | under 10 |
| Repo F | about 200 | under 5 | about 20 | about 300 | about 20 |
| Repo G | about 30 | about 40 | under 10 | about 30 | under 10 |
| Repo H | about 1,200 | under 5 | about 50 | about 1,300 | about 50 |
| Repo I | under 10 | under 5 | under 10 | under 10 | under 10 |
| Repo J | about 30 | about 10 | under 10 | about 40 | under 10 |
| Repo K | about 70 | about 10 | about 20 | about 60 | about 10 |
| Repo L | about 70 | about 10 | about 20 | about 60 | about 20 |
| Repo M | about 100 | under 5 | about 200 | about 200 | about 200 |
| Repo N | about 400 | about 5 | about 900 | about 500 | about 400 |
| Repo O | about 60 | about 15 | about 10 | about 60 | under 10 |

<details>
<summary>All 132 organic repos (today)</summary>

| Repo | Kind | Stars | Forks per 1,000 stars | Watchers per 1,000 stars | Issues and PRs per 1,000 stars | Issues and PRs in the last 90 days |
| --- | --- | ---: | ---: | ---: | ---: | ---: |
| 996icu/996.ICU | cause | 277,178 | 75 | 15 | 7.2 | 5 |
| a2aproject/A2A | AI, 2025 | 26,304 | 102 | 9.1 | 70 | 238 |
| addyosmani/agent-skills | AI, 2026 | 110,760 | 104 | 4.9 | 6.6 | 315 |
| ajeetdsouza/zoxide | tool | 40,304 | 23 | 1.9 | 30 | 76 |
| anthropics/claude-code | AI, 2025 | 153,463 | 170 | 5.9 | 674 | 23,275 |
| anthropics/financial-services | AI, 2026 | 40,806 | 140 | 7.2 | 9.4 | 88 |
| anthropics/knowledge-work-plugins | AI, 2026 | 33,178 | 110 | 6.1 | 42 | 678 |
| antirez/ds4 | AI, 2026 | 24,099 | 97 | 7.2 | 53 | 669 |
| antonmedv/countdown | small | 1,302 | 92 | 12 | 34 | 1 |
| antonmedv/walk | small | 3,649 | 30 | 2.7 | 45 | 1 |
| astral-sh/ruff | tool | 50,305 | 51 | 2.7 | 571 | 2,295 |
| astral-sh/uv | tool | 91,212 | 41 | 2.1 | 249 | 2,064 |
| atuinsh/atuin | tool | 32,090 | 32 | 2.3 | 138 | 780 |
| avelino/awesome-go | list | 190,489 | 72 | 15 | 34 | 285 |
| awesome-selfhosted/awesome-selfhosted | list | 330,686 | 47 | 10 | 0.0 | 0 |
| axios/axios | library | 109,308 | 109 | 11 | 73 | 277 |
| benhoyt/goawk | small | 2,066 | 49 | 18 | 149 | 13 |
| BurntSushi/byteorder | small | 1,101 | 147 | 10.0 | 207 | 3 |
| BurntSushi/ripgrep | tool | 69,464 | 69 | 4.5 | 44 | 81 |
| casey/just | tool | 36,441 | 24 | 2.9 | 97 | 73 |
| charmbracelet/bubbletea | library | 45,518 | 72 | 3.2 | 35 | 133 |
| charmbracelet/glow | tool | 27,818 | 29 | 3.0 | 33 | 73 |
| charmbracelet/gum | tool | 24,545 | 23 | 2.6 | 45 | 65 |
| charmbracelet/pop | small | 2,928 | 26 | 1.7 | 62 | 8 |
| chenglou/pretext | AI, 2026 | 50,918 | 54 | 2.7 | 10 | 312 |
| cli/cli | tool | 46,752 | 197 | 24 | 243 | 771 |
| cloudflare/security-audit-skill | AI, 2026 | 48,336 | 62 | 3.7 | 2.3 | 101 |
| codecrafters-io/build-your-own-x | list | 556,420 | 94 | 12 | 3.1 | 88 |
| CSSEGISandData/COVID-19 | data | 28,883 | 619 | 0.1 | 226 | 0 |
| dandavison/delta | tool | 32,588 | 19 | 3.4 | 66 | 95 |
| date-fns/date-fns | library | 36,655 | 56 | 5.5 | 110 | 128 |
| deepseek-ai/DeepEP | AI, 2025 | 10,361 | 146 | 11 | 76 | 91 |
| deepseek-ai/DeepInfer | AI, 2026 | 22,985 | 116 | 2.4 | 9.0 | 207 |
| deepseek-ai/deepseek-harness | AI, 2026 | 262,493 | 121 | 4.2 | 0.0 | 0 |
| deepseek-ai/DeepSeek-R1 | model release | 91,963 | 127 | 6.5 | 8.7 | 24 |
| deepseek-ai/DeepSeek-V3 | model release | 104,617 | 160 | 7.0 | 14 | 192 |
| deepseek-ai/FlashMLA | AI, 2025 | 13,194 | 94 | 7.9 | 18 | 40 |
| donnemartin/system-design-primer | list | 375,701 | 157 | 18 | 3.1 | 72 |
| EbookFoundation/free-programming-books | list | 400,187 | 167 | 24 | 33 | 179 |
| f/prompts.chat | list | 174,656 | 128 | 9.6 | 6.6 | 61 |
| facebook/astryx | AI, 2026 | 13,759 | 82 | 5.7 | 621 | 4,114 |
| fastapi/fastapi | library | 104,214 | 97 | 7.4 | 97 | 390 |
| FoundationAgents/OpenManus | AI, 2025 | 58,789 | 173 | 7.1 | 23 | 52 |
| garrytan/gstack | AI, 2026 | 137,817 | 148 | 5.6 | 25 | 1,102 |
| ghostty-org/ghostty | tool | 62,617 | 60 | 3.4 | 130 | 840 |
| gin-gonic/gin | library | 89,432 | 98 | 15 | 54 | 142 |
| github/github-mcp-server | AI, 2025 | 33,981 | 154 | 12 | 88 | 477 |
| google-gemini/gemini-cli | AI, 2025 | 108,548 | 137 | 5.5 | 256 | 1,367 |
| google-research/bert | model release | 40,040 | 241 | 0.1 | 34 | 0 |
| google-research/harness2 | AI, 2026 | 20,112 | 98 | 4.5 | 7.1 | 142 |
| google/adk-python | AI, 2025 | 22,014 | 193 | 6.9 | 304 | 1,211 |
| google/skills | AI, 2026 | 21,594 | 85 | 6.3 | 11 | 40 |
| hacksider/Deep-Live-Cam | AI | 97,249 | 146 | 5.8 | 17 | 62 |
| helix-editor/helix | tool | 46,703 | 83 | 4.8 | 280 | 276 |
| huggingface/open-r1 | AI, 2025 | 26,479 | 93 | 10 | 26 | 12 |
| huggingface/smolagents | AI, 2025 | 30,078 | 105 | 4.6 | 97 | 474 |
| jackfrued/Python-100-Days | course | 187,466 | 297 | 32 | 6.1 | 23 |
| jesseduffield/lazygit | tool | 83,430 | 38 | 4.0 | 66 | 265 |
| junegunn/fzf | tool | 83,797 | 56 | 5.3 | 53 | 61 |
| junegunn/limelight.vim | small | 2,453 | 24 | 4.9 | 33 | 1 |
| jwasham/coding-interview-university | list | 363,424 | 234 | 24 | 4.4 | 13 |
| karpathy/autoresearch | AI, 2026 | 98,618 | 138 | 7.4 | 5.3 | 13 |
| kattgu7/Anti-996-License | cause | 1,278 | 47 | 20 | 50 | 0 |
| kdeldycke/awesome-falsehood | small | 27,800 | 23 | 13 | 9.3 | 9 |
| kepano/obsidian-skills | AI, 2026 | 50,159 | 71 | 4.8 | 2.8 | 22 |
| langchain-ai/langchain | AI | 148,506 | 168 | 6.2 | 240 | 2,138 |
| langchain-ai/openwiki | AI, 2026 | 17,468 | 74 | 3.0 | 59 | 540 |
| mattpocock/skills | AI, 2026 | 303,979 | 84 | 5.2 | 3.9 | 571 |
| meta-llama/llama | model release | 59,629 | 164 | 9.0 | 23 | 16 |
| microsoft/generative-ai-for-beginners | course | 122,220 | 531 | 8.3 | 9.9 | 112 |
| microsoft/playwright-mcp | AI, 2025 | 38,663 | 86 | 3.9 | 46 | 97 |
| microsoft/RustTraining | AI, 2026 | 14,966 | 78 | 5.7 | 8.4 | 20 |
| mistralai/mistral-inference | model release | 10,822 | 98 | 11 | 25 | 0 |
| mitchellh/libxev | small | 3,592 | 56 | 9.2 | 71 | 27 |
| MoonshotAI/Kimi-K2 | model release | 11,111 | 82 | 8.7 | 12 | 4 |
| nalgeon/redka | small | 4,566 | 30 | 7.4 | 12 | 1 |
| nalgeon/sqlean | small | 4,381 | 37 | 12 | 37 | 10 |
| neovim/neovim | tool | 103,183 | 70 | 9.9 | 398 | 1,458 |
| nilbuild/developer-roadmap | list | 370,474 | 122 | 19 | 26 | 310 |
| nlohmann/json | library | 50,807 | 148 | 14 | 90 | 569 |
| NVIDIA/OpenShell | AI, 2026 | 20,061 | 99 | 3.6 | 223 | 2,192 |
| nytimes/covid-19-data | data | 6,969 | 485 | 45 | 100 | 1 |
| ollama/ollama | AI | 184,886 | 101 | 5.5 | 102 | 1,861 |
| open-webui/open-webui | AI | 156,843 | 146 | 4.3 | 147 | 4,285 |
| openai/codex | AI, 2025 | 131,021 | 158 | 5.0 | 425 | 20,922 |
| openai/codex-plugin-cc | AI, 2026 | 34,601 | 69 | 3.2 | 23 | 260 |
| openai/gpt-2 | model release | 25,010 | 238 | 25 | 15 | 0 |
| openai/gpt-oss | AI, 2025 | 20,493 | 107 | 6.2 | 16 | 81 |
| openai/math | AI, 2026 | 15,532 | 117 | 22 | 0.1 | 2 |
| openai/openai-agents-python | AI, 2025 | 30,302 | 163 | 7.4 | 173 | 1,356 |
| openai/symphony | AI, 2026 | 27,829 | 104 | 6.3 | 2.9 | 5 |
| openai/whisper | AI | 111,334 | 121 | 7.1 | 4.0 | 61 |
| owid/covid-19-data | data | 5,660 | 629 | 26 | 458 | 0 |
| pallets/flask | library | 75,091 | 228 | 28 | 76 | 51 |
| pingdotgg/t3code | AI, 2026 | 30,458 | 267 | 2.5 | 764 | 18,581 |
| psf/requests | library | 54,720 | 221 | 24 | 133 | 34 |
| public-apis/public-apis | list | 492,401 | 111 | 9.6 | 16 | 1,634 |
| punkpeye/awesome-mcp-servers | list | 96,904 | 191 | 0.5 | 170 | 6,450 |
| pydantic/pydantic | library | 29,201 | 106 | 6.0 | 396 | 536 |
| QwenLM/Qwen3-TTS | AI, 2026 | 14,028 | 130 | 5.1 | 13 | 23 |
| react/react | library | 251,875 | 205 | 26 | 140 | 0 |
| ripienaar/free-for-dev | list | 140,068 | 106 | 13 | 34 | 371 |
| run-llama/liteparse | AI, 2026 | 12,919 | 69 | 2.9 | 40 | 143 |
| rxi/lite | small | 8,240 | 47 | 13 | 38 | 2 |
| rxi/log.c | small | 3,568 | 196 | 17 | 16 | 0 |
| rxi/microui | small | 6,916 | 61 | 13 | 11 | 0 |
| serde-rs/serde | library | 10,916 | 88 | 6.5 | 281 | 25 |
| sharkdp/bat | tool | 60,950 | 60 | 4.1 | 65 | 233 |
| sharkdp/pastel | small | 6,521 | 21 | 4.0 | 50 | 8 |
| Significant-Gravitas/AutoGPT | AI | 187,776 | 244 | 8.2 | 78 | 2,287 |
| simonw/files-to-prompt | small | 2,797 | 66 | 9.3 | 30 | 6 |
| simonw/shot-scraper | small | 2,597 | 52 | 3.5 | 82 | 10 |
| sindresorhus/awesome | list | 525,114 | 72 | 16 | 5.5 | 18 |
| spf13/cobra | library | 44,724 | 116 | 8.1 | 56 | 89 |
| starship/starship | tool | 60,462 | 45 | 3.3 | 115 | 181 |
| Stirling-Tools/Stirling-PDF | app | 95,076 | 112 | 3.1 | 88 | 1,465 |
| tidwall/btree | small | 1,232 | 91 | 8.1 | 47 | 0 |
| tokio-rs/tokio | library | 33,524 | 155 | 9.8 | 235 | 391 |
| torvalds/linux | mirror | 253,569 | 266 | 33 | 0.0 | 0 |
| trimstray/the-book-of-secret-knowledge | list | 253,445 | 58 | 11 | 1.7 | 25 |
| twitter/the-algorithm | news | 73,988 | 179 | 5.5 | 23 | 26 |
| vercel-labs/agent-browser | AI, 2026 | 44,638 | 69 | 2.3 | 49 | 597 |
| vincentsch/explainroo | small, 2026 | 653 | 70 | 3.1 | 61 | 40 |
| vinta/awesome-python | list | 330,449 | 88 | 19 | 8.0 | 118 |
| wuhan2020/wuhan2020 | cause | 5,908 | 140 | 40 | 73 | 0 |
| xai-org/grok-1 | model release | 52,220 | 163 | 12 | 1.8 | 2 |
| xai-org/x-algorithm | AI, 2026 | 33,801 | 162 | 8.9 | 7.7 | 260 |
| yoheinakajima/babyagi | AI | 22,356 | 127 | 14 | 19 | 5 |
| ytdl-org/youtube-dl | tool | 141,399 | 75 | 15 | 225 | 14 |
| zellij-org/zellij | tool | 35,840 | 42 | 3.1 | 145 | 337 |
| zserge/fenster | small | 1,583 | 55 | 15 | 30 | 1 |
| zserge/jsmn | small | 4,203 | 194 | 34 | 59 | 4 |

</details>

<details>
<summary>The 50 organic repos picked to have few forks</summary>

Forks per 1,000 stars on 2026-10-26: ducaale/xh (18), sharkdp/hyperfine (19), Wilfred/difftastic (20), charmbracelet/vhs (23), eza-community/eza (23), bootandy/dust (24), sxyazi/yazi (24), Canop/broot (25), dalance/procs (25), sharkdp/fd (27), sharkdp/hexyl (27), ClementTsang/bottom (28), orf/gping (29), imsnif/bandwhich (29), ast-grep/ast-grep (30), BurntSushi/xsv (30), jesseduffield/lazydocker (32), rxhanson/Rectangle (34), gitui-org/gitui (35), tauri-apps/tauri (37), fastfetch-cli/fastfetch (37), zen-browser/desktop (42), kovidgoyal/kitty (44), XAMPPRocky/tokei (47), LadybirdBrowser/ladybird (48), upscayl/upscayl (52), hyprwm/Hyprland (54), junegunn/vim-plug (54), alacritty/alacritty (56), localsend/localsend (56), microsoft/PowerToys (62), wezterm/wezterm (62), ventoy/Ventoy (62), iina/iina (64), gorhill/uBlock (64), files-community/Files (65), flameshot-org/flameshot (66), AppFlowy-IO/AppFlowy (78), tldr-pages/tldr (86), warpdotdev/warp (87), yt-dlp/yt-dlp (88), mpv-player/mpv (96), ShareX/ShareX (99), zed-industries/zed (122), darkreader/darkreader (123), jgm/pandoc (125), brave/brave-browser (136), obsproject/obs-studio (139), ohmyzsh/ohmyzsh (150), rustdesk/rustdesk (158).

</details>

## What --deep adds for maintainers

GitHub lets people with push access see who starred their repo. With
`--deep`, StarTrust reads that list, 100 accounts per request. For each
busy day it counts two kinds of accounts. New accounts were created at most
30 days before they starred. Empty accounts have no followers and no public
repositories. An account can be both. StarTrust compares these counts with
all of the repo's stargazers. A day needs at least 30 stargazers for its
own line, so a small repo only gets the counts for all of them.

The counts are shown as evidence, but they do not change the risk yet. New
GitHub users do star projects they hear about, and there are not enough
known cases to say what share is normal on a launch day.

## Why are some numbers not used?

Forks, watchers and issues are shown in every report, but they never
change the risk. See [forks, watchers and
issues](#forks-watchers-and-issues).

StarTrust also does not use the share of stars on the single biggest day.
A flagged repo with a big campaign day got under 10% of its stars on that
day, and codex got 6.9% at its launch, so that number cannot tell them
apart. Comparing each day with the repo's own normal day can.
