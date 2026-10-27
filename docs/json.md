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
  "biggest_day": { "day": "2025-07-09T00:00:00Z", "stars": 223, "...": "..." },
  "unusual_spikes": 0,
  "days": [
    {
      "day": "2025-07-09T00:00:00Z",
      "stars": 223,
      "normal_day": 26,
      "times_normal": 8.6,
      "spike": "",
      "accounts": {
        "day": "2025-07-09T00:00:00Z",
        "github_stars": 223,
        "archived": 126,
        "window_recorded": 0.98,
        "checked": 126,
        "one_off": 2,
        "judged": true
      }
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
    "archive_queries": 3,
    "token": true
  }
}
```

A day that could not be judged looks like this. Its `checked` and
`one_off` stay 0, because GH Archive had too little data for a share that
means anything:

```json
"accounts": {
  "day": "2026-10-18T00:00:00Z",
  "github_stars": 6066,
  "archived": 113,
  "window_recorded": 0.68,
  "checked": 0,
  "one_off": 0,
  "judged": false,
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
- `forks`, `watchers` and `issues_and_prs_90_days` show how much the repo
  is used. They are context and never change the risk. Most repos flagged
  for bought stars had normal numbers here, so normal numbers do not clear
  a repo. Every organic repo in StarTrust's comparison had at least 10
  forks per 1,000 stars, but few forks alone mean nothing, because tools
  such as `composer thanks` star library repos automatically ([how it
  works](how-it-works.md#forks-watchers-and-issues)).
- For a share of one-off accounts, read `days[].accounts` only where
  `judged` is `true`. Then `one_off` out of `checked` is the share. On all
  other days, ignore `checked` and `one_off`. There, `problem` or
  `provisional` says why the day was not judged.

## Fields

| Field | Meaning |
| --- | --- |
| `risk` | `low`, `medium`, `high` or `unknown`. See [risk levels](how-it-works.md#risk-levels). |
| `verdict` | The verdict that the terminal shows first. It says what the check saw, in a few sentences. |
| `headline` | The result in a few words, such as `no unusual pattern`, `no unusual timing`, `1 unusual spike`, `more one-off accounts`, `unusual pattern`, `too new to judge` or `not enough data`. The badge shows it. |
| `forks` | Forks of the repo, from GitHub's repo summary. |
| `watchers` | People who get GitHub's notifications for the repo. |
| `issues_and_prs_90_days` | Issues and pull requests opened in the last 90 days, from one GitHub search. For a repo younger than 90 days, all of them. `null` when it was not counted, for example when the search limit of 10 a minute without a token was used up. Then `sources.issues_problem` says why, unless Ctrl-C stopped the check first. |
| `accounts_checked` | `all`, `some` or `none`. It says whether StarTrust could check the accounts behind the busy days that needed it. With `none`, the verdict rests on the timing of the stars. |
| `findings` | The evidence behind the verdict, one sentence each. |
| `limits` | What the check could not see. |
| `days` | Up to five of the busiest days, biggest first, at least 7 days apart. |
| `days[].day` | The date on GitHub's calendar (US Pacific time), written as midnight UTC. |
| `days[].normal_day` | Median stars per day in the 28 days before. |
| `days[].times_normal` | Stars that day divided by the normal day (a normal day under 1 counts as 1). |
| `days[].spike` | `launch`, `unusual`, `spike`, or empty when the day is not part of a spike. |
| `days[].accounts` | The GH Archive check for that day, when it ran. |
| `accounts.archived` | Accounts GH Archive saw starring that day. |
| `accounts.window_recorded` | Stars GH Archive recorded across GitHub per day in the 30 days before and after, compared with the week of the day. Under 1 means that GH Archive recorded the weeks around the day more thinly. For a provisional day it covers only the days so far. |
| `accounts.checked` | Accounts in the sample (at most 1,000). 0 on a day with a `problem`. |
| `accounts.one_off` | Checked accounts with no other public stars recorded in GH Archive within 30 days before or after. 0 on a day with a `problem`. |
| `accounts.judged` | `true` when the share counts. Only then is `one_off` out of `checked` worth reading. |
| `accounts.problem` | Set when GH Archive had too little data to judge the day, on the day or in the weeks around it. |
| `accounts.provisional` | `true` while the 30 days after the day are not over. The share does not count until then. |
| `days[].profiles` | With `--deep`: `accounts`, `new`, `empty` and `new_or_empty` counts for that day. An account can be both new and empty. |
| `days[].explanations` | With `--explain`: Hacker News posts (`kind: hacker_news`) and releases (`kind: release`) near the day. |
| `unusual_spikes` | How many unusual spikes the history has, cleared or not. |
| `burst` | The burst window and daily rates, or `null`. |
| `history_partial` | `true` when only part of the history was read. GitHub may have stopped answering, or the check hit `--max-requests` or Ctrl-C. |
| `sources.history` | `complete`, `partial` or `unavailable`. |
| `sources.archive` | `used`, `partial`, `unavailable`, `not_needed` (no day was big enough) or `off` (`--no-archive`). |
| `sources.stargazers` | `listed`, `partial`, `hidden` or `off` (without `--deep`). |
| `sources.*_problem` | The reason a source was partial or unavailable. |

## With --details

`--details` adds a `details` object:

| Field | Meaning |
| --- | --- |
| `details.history` | `start` (the first day) and `daily`, the stars of each day from then on. |
| `details.spikes` | Every spike with its dates and its biggest day, and whether it is a launch or unusual. |
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
