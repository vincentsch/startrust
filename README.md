# StarTrust

**Check if a GitHub repo's stars look bought. It works on any public repo and needs no account.**

StarTrust is a command-line tool. You give it a GitHub repo, and it tells
you in a sentence or two whether anything about the repo's stars looks
unusual. It draws the stars over time in your terminal and lists the
busiest days. For the biggest of these days, it checks whether the accounts
that starred also star other repos.

[Quickstart](docs/quickstart.md) · [How the verdict works](docs/how-it-works.md) · [Limits](docs/limits.md) · [CI and badge](docs/ci-and-badge.md) · [AGENTS.md](AGENTS.md)

## Set it up with your coding agent

Paste this into your coding agent, such as Claude Code or Codex:

```text
Install StarTrust from https://github.com/vincentsch/startrust (read its
AGENTS.md and follow the steps), then check whether the stars of
<owner/repo> look bought and explain the result to me.
```

The agent installs StarTrust and checks the repo for you. It doesn't need
a GitHub token. [AGENTS.md](AGENTS.md) has the steps it follows.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/vincentsch/startrust/main/install.sh | sh
```

The installer works on Linux and macOS, and on Windows in Git Bash. It
checks the download's SHA-256 checksum before it installs anything. With Go
1.27 or newer you can also run:

```sh
go install github.com/vincentsch/startrust/cmd/startrust@latest
```

## Try it

```sh
startrust check junegunn/fzf
```

This is the output for fzf, a popular fuzzy finder whose stars have grown
steadily for 13 years:

```text
junegunn/fzf · 83,802 stars · created 2013-10-23
https://github.com/junegunn/fzf

No unusual pattern. The stars arrived steadily, and the biggest day had 223
stars, 9x a normal day.
Risk: low

Activity
  4,699 forks (56 per 1,000 stars)
    445 watchers
     61 issues and pull requests in the last 90 days
  In StarTrust's comparison of 182 organic repos, every one had at least 10
  forks per 1,000 stars. Watchers and issues vary a lot with the kind of repo.
  But most repos flagged for bought stars also had normal numbers here, so
  normal numbers do not prove that the stars are real. Few forks alone also mean
  nothing, because tools such as `composer thanks` star library repos
  automatically.

Stars per quarter, 2013-10 to 2026-10
2,930 ┤                                         ▂▅▁▁  ▃ █▂
      │                          ▂          ▃   ████▆▄█▅██▂
      │                ▅  ▂ ▁   ▃█    ▁ ▄█▃ █  ▃███████████
      │                █  █▁█▆████▅████▇███▆█▇▄████████████
      │             ▂▃ █▃▄█████████████████████████████████
      │        ▁▅ ▂▁██▃████████████████████████████████████
      │     ▁▅▆██▇█████████████████████████████████████████▅
      │▂▃▇▅▄████████████████████████████████████████████████
      └─┬───┬───┬───┬───┬───┬───┬───┬───┬───┬───┬───┬───┬───
        2014    2016    2018    2020    2022    2024    2026
                       5  3                 2      4  1

Busiest days, at least a week apart
  1  2025-07-09  223 stars   9x normal         2% one-off accounts
  2  2023-03-23  171 stars   8x normal         3% one-off accounts
  3  2018-08-31  156 stars  11x normal  spike  2% one-off accounts
  4  2024-10-27  153 stars   7x normal
  5  2017-10-20  149 stars  12x normal  spike

What stands out
- Accounts with no other public stars recorded in GH Archive within 30 days: 2%
  on 2025-07-09, 3% on 2023-03-23 and 2% on 2018-08-31. For comparison, organic
  developer tools in the calibration set had up to 21%, and viral lists and
  repos about a cause up to 38%.

Limits
- Anyone can star any repo, so a pattern here says nothing about who arranged
  it.
- GitHub only counts stars that still exist. Stars from deleted accounts are
  missing from the history.
- Account checks use GH Archive, a public third-party record of GitHub events.
  Since June 2025 it has missed a growing share of all stars, and since June
  2026 almost all of them.

Checked 2026-10-26 23:10 CET using 25 GitHub requests and 3 GH Archive queries.
Used your GitHub CLI login, 5,000 requests an hour. Add --details for the full
evidence.
```

The check took under ten seconds. The numbers under the chart point to the
days in the list.

## What it looks at

Since July 2026, GitHub only shows who starred a repo to the people who
maintain it. Everyone can still read the repo's daily star counts, so
StarTrust starts there. It compares each day with a normal day in the four
weeks before and marks the spikes. Launches and Hacker News posts also make
spikes, so a spike alone proves nothing. StarTrust also looks for a burst,
where a repo got at least half of its stars within 90 days and almost none
before or after. Popular projects rarely stop that suddenly.

Then it looks up who starred on the busiest days. For this it uses GH
Archive, a public record of GitHub events. People who star a repo because
they like it usually star other repos around the same time. Accounts used
in star campaigns often don't.

The thresholds come from 46 repos, the calibration set. Of these, 31 are
organic, which means well-known projects with a public story behind their
stars. The other 15 were flagged for fake stars by research or press
reports. For the developer tools in that set, at most 21% of the accounts
behind the busiest days had no other public stars recorded in GH Archive
within 30 days. Viral lists and repos about a cause reached 38%. For the
flagged repos, it was 50% or more on most days.

Every report also shows how many forks and watchers the repo has, and how
many issues and pull requests people opened in the last 90 days. A star
costs one click, while these usually come from people who use the project.
But most repos flagged for bought stars also had normal numbers there. And
some honest repos have almost no forks, because tools such as `composer
thanks` star library repos automatically. So these numbers are context, and
they never change the risk.

StarTrust describes what it saw. It never says that stars were bought, and
when it could not check something, the verdict says so.

Since June 2025, GH Archive has missed a growing share of all stars. So for
a repo created in 2026, StarTrust usually cannot check the accounts, and
timing alone cannot judge a launch. In that case StarTrust says that it
cannot judge the repo (risk "unknown") instead of calling it fine. The
verdict then names the repo's forks and recent issues, and you can weigh
the repo yourself. [Limits](docs/limits.md#gh-archive-has-gaps) has the
numbers by month.

[How the verdict works](docs/how-it-works.md) has every threshold and the
results on all 46 repos. [Limits](docs/limits.md) lists what StarTrust
cannot tell you.

## More things you can do

```sh
startrust check owner/repo --details        # the full evidence
startrust check owner/repo --explain        # match busy days to Hacker News posts and releases
startrust check owner/repo --json           # a compact summary for scripts and agents
startrust check owner/repo --markdown --out stars.md
startrust compare junegunn/fzf charmbracelet/glow
startrust check owner/repo --former-name old-owner/old-name   # a renamed repo: also look up its old name
startrust check your/repo --deep            # maintainers: compare the profiles of your stargazers
```

GitHub allows 60 requests an hour without a token, which is enough for one
or two checks. With a token the limit is 5,000. StarTrust uses the token in
`GITHUB_TOKEN` or `GH_TOKEN`, or your GitHub CLI login. The last line of the
report says which one it used. Add `--no-token` to check without one.

For CI there is a GitHub Action, and `--fail-on` sets the exit code by risk.
StarTrust can also make a Shields.io badge. See [CI and
badge](docs/ci-and-badge.md).

## Docs

- [Quickstart](docs/quickstart.md)
- [How the verdict works](docs/how-it-works.md)
- [Limits](docs/limits.md)
- [CI and badge](docs/ci-and-badge.md)
- [JSON output](docs/json.md)
- [FAQ](docs/faq.md) and [troubleshooting](docs/troubleshooting.md)
- [Command reference](docs/reference/index.md)

[CONTRIBUTING.md](CONTRIBUTING.md) explains how to build and test it.
Report security issues through [SECURITY.md](SECURITY.md).

Built with [rungrad](https://rungrad.com).

## License

[MIT](LICENSE). Made by [Vincent Schmalbach](https://www.vincentschmalbach.com/).
