# JSON output

`startrust check owner/repo --json` prints one JSON object. Scripts and
coding agents can rely on its layout. New fields can be added, but a change
that breaks readers raises `schema_version`.

`report --json` prints the same object for a saved check. `compare --json`
prints `{"repositories": [...], "failed": [...]}`, with one object per repo
and the repos that could not be checked.

## The summary

This is the layout without `--details`, from a check of fzf
(shortened):

```json
{
  "schema_version": 1,
  "repository": "junegunn/fzf",
  "url": "https://github.com/junegunn/fzf",
  "former_names": [],
  "checked_at": "2026-10-26T22:10:56Z",
  "created_at": "2013-10-23T16:04:23Z",
  "stars": 83802,
  "forks": 4699,
  "watchers": 445,
  "issues_and_prs_90_days": 61,
  "archived": false,
  "risk": "low",
  "verdict": "No unusual pattern. The stars arrived steadily, and the biggest day had 223 stars, 9x a normal day.",
  "headline": "no unusual pattern",
  "findings": ["Accounts with no other public stars recorded in GH Archive within 30 days: 2% on 2025-07-09, ..."],
  "limits": ["Anyone can star any repo, ..."],
  "accounts_checked": "all",
  "days_to_check": 3,
  "days_checked": 3,
  "rename_suspected": false,
  "biggest_day": { "day": "2025-07-09T00:00:00Z", "stars": 223, "...": "..." },
  "unusual_spikes": 0,
  "days": [
    {
      "day": "2025-07-09T00:00:00Z",
      "stars": 223,
      "normal_day": 26,
      "times_normal": 8.6,
      "spike": "",
      "judged": true,
      "accounts": {
        "day": "2025-07-09T00:00:00Z",
        "github_stars": 223,
        "archived": 126,
        "window_recorded": 0.98,
        "checked": 126,
        "one_off": 2
      }
    },
    {
      "day": "2024-10-27T00:00:00Z",
      "stars": 153,
      "normal_day": 23.5,
      "times_normal": 6.5,
      "spike": "",
      "judged": false,
      "accounts": null
    }
  ],
  "burst": null,
  "history_from": "2013-10-23T00:00:00Z",
  "history_to": "2026-10-26T00:00:00Z",
  "history_partial": false,
  "sources": {
    "history": "complete",
    "archive": "used",
    "stargazers": "off",
    "explain": "off",
    "github_requests": 25,
    "github_rate_limit": { "limit": 5000, "remaining": 4976, "resets_at": "2026-10-26T23:10:00Z" },
    "archive_queries": 3,
    "token_source": "gh"
  }
}
```

The second day was not checked, so it has `"judged": false` and
`"accounts": null`. Every day has both fields.

A day that was checked but could not be judged looks like this. Its
`checked` and `one_off` stay 0, because GH Archive had too little data for
a share that means anything:

```json
"judged": false,
"accounts": {
  "day": "2026-10-18T00:00:00Z",
  "github_stars": 6066,
  "archived": 113,
  "window_recorded": 0.68,
  "checked": 0,
  "one_off": 0,
  "problem": "GH Archive has only 113 of the 6,066 stars from this day",
  "provisional": true
}
```

## Which fields to read

- `risk` and `headline` give the result. `verdict` says it in a sentence.
- `risk` is `unknown` when StarTrust could not judge the repo. That is not
  a pass. Read `verdict` and `limits` to see what was missing.
- `accounts_checked` says whether the accounts behind the busy days were
  checked. With `none`, the verdict rests on the timing of the stars.
  `days_checked` out of `days_to_check` gives the numbers. Show them with
  the risk, such as "low, accounts checked on 1 of 3 busy days", because a
  low risk with fewer days checked is a weaker result.
- `forks`, `watchers` and `issues_and_prs_90_days` show how much the repo
  is used. They are context and never change the risk. Most repos flagged
  for bought stars had normal numbers here, so normal numbers do not clear
  a repo. Every organic repo in StarTrust's comparison had at least 10
  forks per 1,000 stars, but few forks alone mean nothing, because tools
  such as `composer thanks` star library repos automatically ([how it
  works](how-it-works.md#forks-watchers-and-issues)).
- `rename_suspected` is `true` when GH Archive has no stars at all for a
  busy day from before June 2026. The repo may have had another name then.
  One of the `limits` gives the command to run with `--former-name`. See
  [the repo was renamed](troubleshooting.md#the-repo-was-renamed).
- For a share of one-off accounts, read `days[].accounts` only where
  `days[].judged` is `true`. Then `one_off` out of `checked` is the share.
  On all other days, ignore `checked` and `one_off`. When `accounts` is
  `null`, the accounts were not checked. Otherwise `problem` or
  `provisional` says why the day was not judged.

## Fields

### The repo and the result

| Field | Meaning |
| --- | --- |
| `schema_version` | `1`. It goes up when the layout changes in a way that breaks readers. |
| `repository` | The repo as `owner/name`, spelled the way GitHub spells it now. A renamed repo shows its new name. |
| `url` | The repo's page on GitHub. |
| `former_names` | Earlier names of the repo that GitHub forwards to it: the name you checked, when it was an old one, and every `--former-name`. The account check looked them up in GH Archive too. Empty when there are none. |
| `checked_at` | When the check ran, in UTC. |
| `created_at` | When the repo was created on GitHub. |
| `stars` | The star count from GitHub's repo summary. |
| `forks` | Forks of the repo, from GitHub's repo summary. |
| `watchers` | People who get GitHub's notifications for the repo. |
| `issues_and_prs_90_days` | Issues and pull requests opened in the last 90 days, from one GitHub search. For a repo younger than 90 days, all of them. `null` when it was not counted, for example when the search limit of 10 a minute without a token was used up. Then `sources.issues_problem` says why, unless Ctrl-C stopped the check first. |
| `archived` | `true` when the repo is archived on GitHub, which makes it read-only. This has nothing to do with GH Archive. |
| `risk` | `low`, `medium`, `high` or `unknown`. See [risk levels](how-it-works.md#risk-levels). |
| `verdict` | The verdict that the terminal shows first. It says what the check saw, in a few sentences. |
| `headline` | The result in a few words, such as `no unusual pattern`, `no unusual timing`, `1 unusual spike`, `more one-off accounts`, `unusual pattern`, `too new to judge` or `not enough data`. The badge shows it. |
| `findings` | The evidence behind the verdict, one sentence each. |
| `limits` | What the check could not see. |
| `accounts_checked` | `all`, `some` or `none`. It says whether StarTrust could check the accounts behind the busy days that needed it. With `none`, the verdict rests on the timing of the stars. |
| `days_to_check` | How many busy days needed an account check: up to 3 of the busiest days with at least 50 stars, and the biggest day of each unusual spike. 0 when no day was big enough. |
| `days_checked` | How many of those days got an account check that counts (`judged` is `true`). The reports print it next to the risk, such as "accounts checked on 1 of 3 busy days". |
| `rename_suspected` | `true` when GH Archive has no stars at all for the repo, under any of the names it was asked for, on a busy day from before June 2026. GH Archive files stars under the name a repo had at the time, so the repo may have had another name then. A line in `limits` gives the command to run. Since June 2026 GH Archive misses almost every star, so later days never set it. |
| `biggest_day` | The day with the most stars, laid out like an entry of `days`. It is usually also in `days`. `null` for a repo without stars. |
| `unusual_spikes` | How many unusual spikes the history has, cleared or not. |
| `days` | Up to five of the busiest days, biggest first, at least 7 days apart. |
| `burst` | The burst window and daily rates, or `null`. |
| `history_from` and `history_to` | The first and the last day of the star history that was read. |
| `history_partial` | `true` when only part of the history was read. GitHub may have stopped answering, or the check hit `--max-requests` or Ctrl-C. |

### Busy days

Each entry of `days` and the `biggest_day` has these fields:

| Field | Meaning |
| --- | --- |
| `day` | The date on GitHub's calendar (US Pacific time), written as midnight UTC. |
| `stars` | The stars GitHub counts for that day. |
| `normal_day` | Median stars per day in the 28 days before. |
| `times_normal` | Stars that day divided by the normal day, or `null` when the normal day is 0. A ratio against no stars would only repeat the star count. A normal day between 0 and 1 star counts as 1. |
| `spike` | `launch`, `unusual`, `spike`, or empty when the day is not part of a spike. |
| `judged` | `true` when the day's share of one-off accounts counts. Only then is `one_off` out of `checked` in `accounts` worth reading. `false` when the accounts were not checked. It is also `false` when `accounts` has a `problem` or is `provisional`. |
| `accounts` | The GH Archive check for that day, or `null` when the accounts were not checked. See below. |
| `profiles` | With `--deep`: `accounts`, `new`, `empty` and `new_or_empty` counts for that day. An account can be both new and empty. |
| `explanations` | With `--explain`: Hacker News posts (`kind: hacker_news`) and releases (`kind: release`) near the day. |

### The account check of a day

StarTrust checks the accounts behind up to 3 of the busiest days with at
least 50 stars, and behind the biggest day of each unusual spike. For each
of these days, `accounts` has these fields:

| Field | Meaning |
| --- | --- |
| `day` | The same day as above. |
| `github_stars` | The stars GitHub counts for that day. |
| `archived` | Accounts GH Archive saw starring the repo that day. It can be higher than `github_stars` (see below). |
| `window_recorded` | How well GH Archive recorded the weeks around the day. See below. |
| `checked` | Accounts StarTrust checked: all of `archived`, or a sample of 1,000 when there were more. 0 on a day with a `problem`. |
| `one_off` | Checked accounts with no other public stars recorded in GH Archive within 30 days before or after. 0 on a day with a `problem`. |
| `problem` | Set when GH Archive had too little data to judge the day, on the day or in the weeks around it. |
| `provisional` | `true` while the 30 days after the day are not over. The share does not count until then. |

StarTrust checks at most 1,000 accounts per day. When GH Archive has more
stargazers for a day, StarTrust checks a sample of 1,000 of them, and it
picks the same 1,000 every time. Then `checked` is 1,000 and smaller than
`archived`, and the share of one-off accounts is the share in the sample.
The terminal and Markdown reports mark such a share with "(sample)".

`archived` can be higher than `github_stars`. GitHub counts only the stars
that still exist. When an account is deleted or someone takes a star back,
GitHub's count drops, but GH Archive keeps its record of the star. A day
with more stars in GH Archive than on GitHub counts as fully recorded.
[Limits](limits.md#gh-archive-has-gaps) has these shares by month.

`window_recorded` compares two numbers from GH Archive: the stars it
recorded per day across all of GitHub in the 61 days from 30 days before
to 30 days after the day, and the stars per day in the week around the
day. `1` means that the weeks around the day were recorded as well as its
week. Under 1 means that they were recorded more thinly. Then the other
stars of the day's accounts can be missing too, and the accounts would
look one-off when they are not. StarTrust multiplies the share of the
day's stars that GH Archive has by `window_recorded` (at most 1). It
judges the day only when the result is at least 40%. A provisional day
leaves `window_recorded` out of this, because the weeks after it have not
all happened.

### Sources

`sources` says where the data came from and what was missing. It also
counts the requests the check sent.

| Field | Meaning |
| --- | --- |
| `history` | `complete`, or `partial` when only part of the star history was read. Then `history_problem` says why. |
| `archive` | The account check in GH Archive: `used` (every day that needed it got an answer), `partial` (some days did), `unavailable` (no day did), `not_needed` (no day was big enough) or `off` (`--no-archive`). |
| `stargazers` | The stargazer list for `--deep`: `listed`, `partial`, `hidden` (GitHub does not show it to this token) or `off` (without `--deep`). |
| `explain` | The lookup for `--explain`: `used`, `partial` or `off` (without `--explain`). |
| `github_requests` | GitHub API requests this check sent, retries included. In `compare`, each repo counts its own. |
| `github_rate_limit` | GitHub's hourly limit after the check's last request, from the `X-RateLimit` headers GitHub sends. `limit` is the number of requests an hour, such as 60 without a token. `remaining` says how many were left, and `resets_at` says when the count starts again, in UTC. `null` when GitHub sent no such headers. The search that counts issues has a limit of its own and is not part of it. |
| `archive_queries` | Queries this check sent to GH Archive, at most one per checked day. Days whose answer was in the cache cost none. |
| `token_source` | The GitHub token the check used: `GITHUB_TOKEN`, `GH_TOKEN`, `gh` (the token that `gh auth token` gave, from your GitHub CLI login) or `none`. It is `none` with `--no-token`, and after GitHub rejected the token. |
| `history_problem` | Why the history is partial. |
| `archive_problem` | Why GH Archive did not answer for a day. |
| `stargazer_problem` | Why the stargazer list is `hidden` or `partial`. |
| `explain_problem` | Why part of the `--explain` lookup failed. |
| `token_problem` | Set when GitHub rejected the token. The check then ran without one. |
| `issues_problem` | Why `issues_and_prs_90_days` is `null`. |

A `*_problem` field is left out when there was no problem.

## With --details

`--details` adds a `details` object:

| Field | Meaning |
| --- | --- |
| `details.history` | `start` (the first day) and `daily`, the stars of each day from then on. |
| `details.spikes` | Every spike with its dates and its biggest day, and whether it is a launch or unusual. Its `times_normal` compares the biggest day with the normal day before it, and it is `null` when that normal day is 0. |
| `details.normal_day` | The normal day before the biggest day. |
| `details.recent_daily` | Average stars per day over the last 30 days. |
| `details.profiles` | With `--deep`: the counts for all stargazers. |
| `details.stargazers_listed` | With `--deep`: how many stargazers were listed. |

A check saved with `--json --details --out check.json` can be shown again
with `startrust report check.json`.

## Errors

Errors go to stderr. With `--json` they are also JSON, with an `error`
message and the `exit_code`. The [exit codes](troubleshooting.md#exit-codes)
are stable. After Ctrl-C the exit code is 130, and stdout holds the partial
result when there is one.
