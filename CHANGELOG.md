# Changelog

## Unreleased

First release of StarTrust.

- Check whether any public GitHub repo's stars look bought, with one command and no account.
- Print a verdict in words first, then a terminal chart of stars over time with the busiest days marked.
- Find spikes against each repo's own normal day, and bursts between quiet periods.
- Check the accounts behind the busiest days in GH Archive and report how many have no other public stars recorded there.
- Describe what was seen without guessing who did it, and say when part of the evidence could not be checked.
- Say "Too new to judge" or "Not enough data to judge" (risk unknown) when neither the timing nor the accounts can tell, instead of calling the repo fine.
- Judge a day's accounts only when GH Archive covers that day and the weeks around it, and leave unjudged days out of the one-off counts.
- Calibrate the rules on 46 repos. 31 are organic projects of different kinds, including viral lists and repos about a cause, and 15 were flagged by public research.
- Show how much a repo is used besides its stars, such as its forks and new issues, as context that never changes the risk. As a reference, the report gives the fewest forks per star seen in 182 organic repos. A repo too new to judge gets these numbers in its verdict.
- Match busy days with Hacker News posts and GitHub releases (`--explain`).
- Let maintainers compare the profiles of their own stargazers (`--deep`), and continue after an interruption.
- Compare several repos side by side on one timeline.
- Write Markdown reports for sharing and compact JSON for scripts and coding agents.
- Fail CI with `--fail-on`, or run the check from a GitHub Action.
- Make a Shields.io badge from a saved check.
- Save one file per finished check in a local cache, so `report` and `badge` work offline.
- Work without a GitHub token, and print what was read so far when a limit is reached or you press Ctrl-C.
- Say which GitHub token a check used, and check without one with `--no-token`.
- Look up a renamed repo's old names in GH Archive (`--former-name`), say when GitHub forwarded an old name, and say what to run when a busy day suggests a rename.
- Install checksum-checked binaries for Linux, macOS and Windows on amd64 and arm64.
