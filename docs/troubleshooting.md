# Troubleshooting

Start with the error message. StarTrust says what failed and why. It also
says what you can try next. The sections below cover the common cases.

## "GitHub's rate limit is used up"

Without a token, GitHub allows 60 requests an hour from your IP address.
A check uses about one request for every 30 weeks of the repo's age, plus
two. So a ten-year-old repo needs about 20, however many stars it has. The
last lines of every report say how many requests were left after the check
and when the limit resets. So you know before the next check whether
enough are left. Log in with the GitHub CLI (`gh auth login`) or set
`GITHUB_TOKEN` to get 5,000 an hour. The
message says when the limit resets. When it says "for this token", the
token's own limit is used up, so wait until then.

When the limit runs out halfway through a history, StarTrust still prints
a report for the part it read and says which dates it covers.

## "GitHub rejected the token"

Your GitHub token has expired or was revoked. The message says where
StarTrust found it, such as `GH_TOKEN` or your GitHub CLI login. StarTrust
continues without it and says so under Limits. Run `gh
auth login` again or set a new token. To stop using it right away, add
`--no-token`.

## The report used a token you did not expect

The last lines of the report say which token the check used. "Used your
GitHub CLI login" means that `gh auth token` gave StarTrust a token. The
GitHub CLI also finds a login in your system keyring, even when `gh auth
status` reports none. This happens, for example, in a shell whose HOME
points to an empty folder. Add `--no-token` to check without it.

## "GitHub has no public repository called ..."

Check the spelling. Private repos only work with a token that can read
them. A repo that was renamed still works under its old name, because
GitHub forwards it. The report then shows the current name, and the line
under it says that GitHub forwards the old one.

## "No account check from GH Archive"

The account check uses ClickHouse's free public endpoint. The message says
why it failed:

- "the free public endpoint's hourly limit is used up" means that you read
  as much as the endpoint allows per user and hour. Try again later.
  StarTrust saves the answers for older days, so a second check of the same
  repo costs less.
- With "the endpoint is down" or a timeout, try again later. You can also
  use `--no-archive` to check the repo without the account check.

The rest of the check still runs. Without account data, unusual spikes and
bursts stay at risk "medium". A repo whose timing StarTrust can judge, and
that shows nothing unusual, gets "No unusual pattern in the star timing".
The verdict then says that the accounts could not be checked. A young repo,
or one that got most of its stars at its launch, gets risk "unknown".

## "too few in GH Archive"

GH Archive does not record every star. Since June 2025 it has missed a
growing share, and since June 2026 almost all of them.
[Limits](limits.md#gh-archive-has-gaps) has the numbers by month. StarTrust
only judges a day when the archive has at least 30 of its stargazers and at
least 40% of the stars GitHub counts, on the day and in the weeks around
it. For newer spikes, use `--explain` to look for a Hacker News post or a
release.

When GH Archive has no stars at all for a busy day, the repo may have had
another name then. See [the repo was renamed](#the-repo-was-renamed).

## The repo was renamed

GH Archive files stars under the name a repo had at the time. When a busy
day from before June 2026 has no stars at all in GH Archive, the Limits
part of the report gives a command such as this one:

```sh
startrust check withastro/astro --former-name OLD-OWNER/OLD-NAME
```

Put the repo's old name in place of `OLD-OWNER/OLD-NAME`. You can add
`--former-name` more than once. StarTrust looks up every name in the same
check, so one result covers the whole history. Checking the old name
itself also works, such as `startrust check snowpackjs/astro`, because
GitHub forwards it to the repo. In `compare`, write the repo and its old
name together:

```sh
startrust compare sindresorhus/is withastro/astro --former-name withastro/astro=snowpackjs/astro
```

A check with a former name is a new full check. It uses as many GitHub
requests as the first one, plus one for each name. Without a token, look
at the requests left in the last lines of the first report before you run
it.

GitHub does not list a repo's former names, so StarTrust cannot find them
for you. Old links often show them, for example in the project's changelog
or in blog posts from that time. If
`https://github.com/old-owner/old-name` opens the repo in your browser,
GitHub forwards that name, and StarTrust can use it. GH Archive matches
names exactly, so write the old name with the same capital letters as back
then.

StarTrust only uses a former name that GitHub still forwards to the repo.
For any other name it stops before the check with exit code 1 and a
message such as "--former-name a/b: GitHub has no repo by that name". This
keeps the stars of another repo out of the result. So you can try a likely
guess, such as the same name under an older organization. A wrong guess
costs two requests. In `compare`, only the repo with the wrong name fails,
and the others are still compared.

If nobody knows the old name, the check stays partial. StarTrust has no
other way to check the accounts behind those days. The risk line says how
many busy days were checked, such as "Risk: low, accounts checked on 1 of 3
busy days", and the verdict says which days are missing. Read such a
result as weaker than a fully checked one.

## "Too new to judge" or "Not enough data to judge"

StarTrust could not judge the repo, so the risk is "unknown". Timing alone
cannot tell a launch from a star campaign, and a repo needs about 240 days
of history before StarTrust can find a burst in it. When the accounts also
could not be checked, nothing is left to judge by. For repos created in
2026 this is common, because GH Archive has too few records of their busy
days.

This says nothing good or bad about the repo. `--fail-on` does not fail on
it, and the badge says "too new to judge" or "not enough data". The verdict
names the repo's forks and recent issues, and "Activity" shows them next to
its stars, so you can weigh the repo yourself.

If the verdict says that only part of the star history was read, the check
stopped early. Run it again later, or with a token.

## "GitHub's search did not answer"

StarTrust counts a repo's recent issues and pull requests with one GitHub
search. Without a token, GitHub allows 10 searches a minute, so a `compare`
of many repos can run out. The report then shows "?" for that count, and
the JSON has `null`. The Limits part of the report says why the search
failed. The rest of the check is not affected. Wait a minute and run it
again, or log in with `gh auth login` for 30 searches a minute.

## "provisional"

The busy day is less than about a month old. The account check counts the
stars each account gave in the 30 days before and after the day, and the
days after have not all happened yet. StarTrust shows the share so far but
leaves it out of the risk. The Limits part of the report says from which
day the check counts. Run the same check again after that day.

## "--deep needs push access"

GitHub only shows who starred a repo to people with push access to it.
Make sure StarTrust uses a token of an account with push access.
`gh auth status` shows which account the GitHub CLI uses. For repos you
don't maintain, use the check without `--deep`.

## You pressed Ctrl-C

StarTrust stops and prints a report of what it read so far. The report
says that it is partial, and the risk is usually "unknown". The exit code
is 130. The partial report is not saved, so `report` and `badge` still show
the last full check. StarTrust saves the GH Archive answers it already got,
so running the same command again costs less.

## A --deep run stopped halfway

Run the same command again within a day. StarTrust saved the stargazers it
had listed and continues from there.

## No progress messages

Progress goes to stderr while you wait, but only when both stdout and
stderr are a terminal. It stays off with `--json` or `--quiet`, and when
the output goes to a pipe or a file.

## The chart looks broken

The chart uses Unicode block characters. Use a terminal font that has them
(any common monospace font does). On older Windows consoles, use Windows
Terminal.

## Exit codes

| Code | Meaning |
| --- | --- |
| 0 | The check ran. Read the verdict for the result. |
| 1 | Wrong use, such as a bad repo name or flag. |
| 2 | Something failed that StarTrust could not work around, for example a network error or a file it could not write. The message says what failed. |
| 3 | GitHub rejected the token on a request that needed it. |
| 4 | GitHub refused access. |
| 5 | The repo does not exist, or there is no such file or saved check. |
| 6 | GitHub's rate limit is used up, or `--max-requests` was reached before anything useful was read. |
| 7 | `--fail-on` matched, because the risk is at or above the level you set. An unknown risk never matches. |
| 130 | You pressed Ctrl-C. The report StarTrust printed is partial, and it was not saved. |

`compare` prints the repos it could check and then exits with the code of
the first repo that failed.
