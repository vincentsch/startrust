# Releasing StarTrust

This page describes how to make a release. A release is a `vX.Y.Z` tag.
Pushing the tag runs `.github/workflows/release.yml`. It runs the full CI
first, and then publishes six binaries with checksums and build provenance
(a signed record of how each binary was built).

## Check the candidate locally

Start from a clean checkout. You need:

- the Go version in `go.mod`
- Python 3 and Make
- a C compiler for the race detector

```sh
make test race lint docs-check links conformance
git diff --check
make live    # talks to the real GitHub API and GH Archive
```

`make live` checks junegunn/fzf, and also the repo in
`STARTRUST_LIVE_FLAGGED` if you set that variable. It needs network access
and uses about 25 GitHub requests for fzf. Set `GITHUB_TOKEN` so you do not
run into the limit of 60 requests an hour.

Look through `git ls-files` for anything private, such as tokens or paths
from your own machine. Search for tokens before you publish:

```sh
git grep -n -E 'gh[pousr]_[A-Za-z0-9]{20,}|github_pat_'
```

## Update the changelog

Rename `## Unreleased` in `CHANGELOG.md` to `## 0.1.0 - YYYY-MM-DD` with the
release date, and start a new empty `## Unreleased` section above it.
Commit that change before tagging. The tag is `v0.1.0`; the binary reports
`0.1.0`. Check the notes the release will use:

```sh
python3 scripts/release-notes.py 0.1.0
```

The release fails when the dated section is missing.

## Build and try the binaries

```sh
make build-all VERSION=0.1.0 COMMIT="$(git rev-parse HEAD)"
```

`dist/` then holds `startrust-linux-amd64`, `startrust-linux-arm64`,
`startrust-darwin-amd64`, `startrust-darwin-arm64`,
`startrust-windows-amd64.exe`, `startrust-windows-arm64.exe` and
`checksums.txt`. Run the native one:

```sh
./dist/startrust-linux-amd64 --version
./dist/startrust-linux-amd64 check junegunn/fzf
```

Check the others with `file`. Cross-compiling does not prove that a
binary runs, so the CI runs the tests natively on Linux, macOS and Windows.

`go test ./scripts` installs from a fake local release with `install.sh`
and checks that a wrong checksum is refused.

## Publish

Only when the release is approved:

1. Make sure CI is green on the commit you want to release.
2. Tag it and push the tag:

   ```sh
   git tag -a v0.1.0 -m "StarTrust 0.1.0"
   git push origin v0.1.0
   ```

3. Watch the Release run in the repo's Actions tab. It creates the GitHub
   release with the notes from `CHANGELOG.md`. It uploads the six binaries
   and `checksums.txt`, and it attests the binaries.

## After publishing

Check the release from a clean machine:

```sh
curl -fsSL https://raw.githubusercontent.com/vincentsch/startrust/main/install.sh | sh
startrust --version
startrust check junegunn/fzf
gh attestation verify ~/.local/bin/startrust --repo vincentsch/startrust
```

Then try the action in a test repo with `uses: vincentsch/startrust@v0.1.0`
and check that the job summary shows the report.
