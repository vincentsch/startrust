# Reporting a security issue

Use [GitHub private vulnerability reporting](https://github.com/vincentsch/startrust/security/advisories/new)
for security reports. Do not put exploit details or tokens in public issues.

Include the StarTrust version (`startrust --version`) and your operating
system, and describe the steps to reproduce the problem. Remove tokens from
logs and screenshots before you share them.

## What StarTrust reads and writes

StarTrust only reads, and it changes nothing on GitHub. It sends read-only
requests to:

- the GitHub API
- ClickHouse's public endpoint, for GH Archive
- Hacker News search, only with `--explain`

It looks for a GitHub token in `GITHUB_TOKEN` or `GH_TOKEN`, or asks the
GitHub CLI for one. StarTrust only sends the token to the GitHub API host,
and it refuses redirects to other hosts. It removes the token from error
messages and never writes it to the cache or to reports.

Finished checks are saved in your user cache directory, in a folder only
your account can read. StarTrust refuses to write through a symbolic link
inside that folder. Reports you write with `--out` are normal files that
others on the machine can read, because they are meant for sharing.

Issues worth reporting include:

- a token that reaches any other host or file
- a write outside the cache or the `--out` path
- a crafted API answer that makes StarTrust run code or read local files
