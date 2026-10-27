# Contributing to StarTrust

StarTrust is a Go program. Use the Go version in [go.mod](go.mod);
`GOTOOLCHAIN=auto` can fetch it. Release builds use `CGO_ENABLED=0` and need
nothing else installed.

## Build and check a change

```sh
make build
./bin/startrust check junegunn/fzf
make test race lint docs-check links conformance
```

On Windows without Make, run `go build ./...`, then `go test ./...` and
`go vet ./...`. CI runs the tests on Linux, macOS and Windows.

## How the code is laid out

| Package | What it does |
| --- | --- |
| `cmd/startrust` | The entry point. |
| `internal/cli` | Commands, flags and output files. |
| `internal/check` | Runs a check and decides the verdict. |
| `internal/stars` | Turns daily counts into spikes and bursts. It does no I/O. |
| `internal/github` | The GitHub REST and GraphQL client. |
| `internal/archive` | The GH Archive query on ClickHouse. |
| `internal/explain` | Hacker News search for `--explain`. |
| `internal/render` | Terminal text, Markdown and the chart. |
| `internal/cache` | Saved checks on disk. |

## Tests

The fast tests need no network. They replay GitHub answers that were
captured once:

- `internal/github/testdata/glow-history.json` holds 13 pages of glow's
  star history, with the `Link` headers GitHub sent.
- `internal/stars/testdata/` holds the full star histories of the 46
  calibration repos, with the GH Archive answers for their busiest days.
  Repos that research flagged are stored under made-up names such as
  `flagged_a/repo`. Their dates are moved and all their counts are scaled
  down by 5 to 15 percent, so they cannot be looked up.
  `internal/check/calibration_test.go` replays each one and checks the
  risk, with and without the account check.
- `internal/cli/testdata/` holds the expected output of a few checks
  (golden files). After a change to the output that you want, run
  `go test ./internal/cli -update` and read the diff.

`make live` checks junegunn/fzf against the real GitHub API and GH Archive.
Run it when you change anything that talks to them. CI runs it weekly.

Every bug fix gets a test that would have caught it.

## Changing the verdict

The thresholds are in these files:

- `internal/stars/analyze.go` for spikes and bursts
- `internal/archive/archive.go` for the account check
- `internal/check/verdict.go` for the risk levels

If you change one, run the calibration again (see [how the verdict
works](docs/how-it-works.md#calibration)). Then update the table there and
the expected results in `internal/check/calibration_test.go`. A new rule
needs results on both flagged and organic repos. Never name a third-party
repo as suspect in the code or the docs.

## Docs

Run `make docs` after you change a command or flag. It rewrites
`docs/reference/`. Update the other guides when the behavior changes.
Write plain English with short sentences, and describe patterns, never
people.

Report security issues through [SECURITY.md](SECURITY.md).
