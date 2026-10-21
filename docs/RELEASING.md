# Package release runbook

Use `scripts/release.py` for compiler/runtime package releases. This process is
separate from application API contract versions and `contracts/releases.json`.
Distribution is GitHub Releases, the public Go tag, and the compiler/plugin package on npm. There are no uploads to
PyPI, Maven Central or NuGet.org.

## Prerequisites

- Git, Python 3.11+ and GitHub CLI (`gh auth login`). On Windows, `py -3` can replace
  `python`; use an installed interpreter rather than the Windows Store alias.
- PowerShell 7, Docker with Linux containers, Go at the pinned toolchain, Bash and
  protoc 33 for local full preparation (including Edition 2023 tests). Windows Git Bash supplies Bash. Use the
  GitHub workflow for the complete pinned CI toolchain instead of installing all
  build tools locally. Runtime builds use the existing Dockerfiles.
- Read access is enough for preparation and dry-run promotion. Only the explicit
  publication command needs repository contents write access. Normal GitHub
  authentication is used; do not put tokens in command arguments or reports.

The current version and counted update targets live in `release.json`.
`python scripts/release.py check` detects stale targets and package metadata.
Version updates touch only the declared targets and the new changelog entry;
historical changelog entries and compatibility statements are preserved. If an
intentional documentation edit changes a managed reference count, update its
target count in the same PR. Package verification/test fixtures derive their
version from the manifest or use an independent fixture version.

## 1. Prepare and review a version PR

Start with a clean checkout of current `main`. The example version is a proposed
next release, not authorization to publish it. Choose the appropriate version and
write a real summary; edit the resulting changelog before committing if needed.

```powershell
git fetch origin
$version = '0.5.0'
git checkout -b "codex/release-v$version" origin/main
$base = git rev-parse HEAD
$date = Get-Date -Format yyyy-MM-dd
python scripts/release.py bump --version $version --commit $base --date $date --summary 'Describe the release changes' --dry-run
python scripts/release.py bump --version $version --commit $base --date $date --summary 'Describe the release changes'
git diff --check
git diff
git add .
git commit -m "Prepare v$version"
git push -u origin HEAD
gh pr create --base main --fill
```

`bump` requires the exact expected HEAD, a clean checkout, a higher canonical
version and no existing public tag or GitHub Release for that version. It checks
all targets before writing any edits. Its dry run only prints the planned files.
It consumes an existing Unreleased changelog section or inserts a new section;
it does not rewrite old release history or deliberately mismatched fixtures.
If a filesystem error interrupts updates, inspect the Git diff and restore only
the generated edits before retrying; it will not silently accept version drift.

Review the PR and all CI results. Merge through the protected branch using the
reviewed head SHA, for example `gh pr merge NUMBER --squash --match-head-commit SHA`.
Fetch the merged commit and use that exact full SHA for subsequent steps.

## 2. Build a candidate without publishing

After the version PR merges:

```powershell
gh workflow run package-release.yml --ref main
```

The existing workflow now has **read-only permissions and no draft/publish job**.
It checks the clean source and unpublished version, runs compiler tests, vet,
source-retention tests, vulnerability scanning, package/license checks, the clean
starter, all 20 combinations, native naming and rollout checks, then uploads
`proto-contract-release-VERSION`. Merging a PR does not dispatch this workflow.

Its summary gives the source commit, run ID, artifact ID and exact dry-run
promotion command. Wait for the entire run to succeed, not just artifact upload.
The artifact expires after 30 days. If it expires, prepare a new candidate and
review the new identity and hashes rather than pretending it is the old one.

The candidate contains the existing installable packages, `manifest.json`,
`SHA256SUMS`, plus `candidate.json` and `RELEASE_NOTES.md`. Metadata records the
version, source commit/tree, clean status, workflow run/attempt, tested bridge
revision and toolchain/build definitions. Notes derive from the version's
changelog entry and include installation and compatibility links. Fixed archive
timestamps and deterministic metadata do not guarantee byte-identical builds
across all machines/ecosystems. Hashes are integrity checks, not signatures.

For a complete **local** dry run after committing a proposed version:

```powershell
python scripts/release.py prepare --commit FULL_SHA --out artifacts/local-candidate
python scripts/release.py publish --version VERSION --commit FULL_SHA --candidate artifacts/local-candidate --dry-run
```

Local preparation invokes the same existing tests and scripts. It produces a
candidate with no workflow identity, which **cannot be published by this tool**.
Choose a fresh output directory; existing outputs are never merged/overwritten.
The low-level `bundle` command is for the workflow after validation, not a
replacement for running the tests.

## 3. Review promotion, then explicitly publish

Use the exact identifiers from the workflow summary:

```powershell
python scripts/release.py publish --version VERSION --commit FULL_SHA --run-id RUN_ID --artifact-id ARTIFACT_ID --dry-run
```

This downloads and checks the candidate and examines existing tags/releases, but
performs no GitHub writes. It rejects failed runs, other workflows/branches/repos,
wrong commits, expired/wrong artifacts, a different run attempt, dirty provenance,
missing/extra/tampered assets, inconsistent versions and conflicting releases.
It verifies the original package entries inside the Go proxy ZIP as well as the
outer asset checksums. Review the artifact's release notes and the dry-run report.

Only when publication is explicitly authorized, run the same command **without
`--dry-run`** and retain its final JSON report:

```powershell
python scripts/release.py publish --version VERSION --commit FULL_SHA --run-id RUN_ID --artifact-id ARTIFACT_ID > artifacts/release-verification.json
```

This promotes the downloaded bytes; it does not rebuild. It creates or verifies
the exact tag, creates a marked draft, uploads missing assets, validates every
uploaded asset, then publishes and marks the release latest. **Creating the public
tag already publishes the Go module**, even while the GitHub Release is a draft.
No tag or release is created by `bump`, `prepare` or a dry run.

Publication retries reuse only a matching tag and a draft/public release carrying
the same candidate marker and notes. Completed uploads are verified and skipped;
no `--clobber`, tag moves, asset replacement or automatic deletion is used. A
conflicting name, target, hash, candidate identity or newer latest release stops
the operation before it overwrites anything. If another maintainer is releasing,
coordinate one publication at a time; server-side conflicts stop rather than
replace their work.

An interrupted/failed upload with a non-uploaded or incorrect asset must be
inspected. Remove only that failed **draft** asset manually if appropriate, then
retry the identical command. If a conflicting draft belongs to another candidate,
resolve ownership explicitly instead of changing its marker to bypass checks.

## 4. Verify and retain evidence

### npm compiler/plugin publication

Starting with the npm distribution, candidate bundles also include `dunkymole-proto-contract-VERSION.tgz`. Publish that exact tested tarball, rather than packing or rebuilding from a checkout. npm publication needs separate explicit authorization and a current npm login (and any registry-required second factor).

```sh
npm publish /absolute/path/to/dunkymole-proto-contract-VERSION.tgz --access public --tag latest
npm view @dunkymole/proto-contract@VERSION name version bin engines dist.integrity --json
```

Then fetch the registry package with `npm pack @dunkymole/proto-contract@VERSION --ignore-scripts` into a fresh directory and compare its SHA-512 integrity and bytes with the approved candidate. Install the public version in a clean project with `--ignore-scripts`, run both compiler and real protoc-plugin consumer checks, and verify the `latest` dist-tag. Never put authentication tokens or one-time codes into tracked files or release reports. GitHub/Go publication and npm publication are separate operations: a failure after either succeeds must be reported and resumed without replacing a published version.

Successful publication freshly downloads the public files, checks exact candidate
bytes, both checksum inventories, public/stable/latest state and the tag target.
It then downloads the normal public Go module and installs both compiler commands
in a fresh Docker container using `proxy.golang.org` and `sum.golang.org`. There
are four bounded attempts with ten-second waits for Go propagation. Do not set
`GONOSUMDB` or compare the test-proxy ZIP with the canonical public module ZIP.

To verify independently later:

```powershell
python scripts/release.py verify --version VERSION --commit FULL_SHA --run-id RUN_ID --out artifacts/fresh-verification > artifacts/fresh-verification.json
```

For releases predating candidate metadata, omit `--run-id` and add `--legacy`.
Keep the JSON report with the run/artifact identity and release URL as maintenance
evidence; attach/link it from your release record if desired. Verification changes
no GitHub state. A nonzero exit after publication does not mean nothing was
published: inspect the release first, address propagation or verification errors,
then retry the same publication/verification command. Never silently repair a
published tag or replace published bytes; use a new version for corrected assets.

## Test the process itself

```powershell
python -m unittest discover -s packaging -p 'test_release*.py'
python scripts/test-release-dry-run.py
```

The second command creates a disposable local Git worktree, bumps the next minor
version, commits it locally, executes full preparation and dry-run promotion, and
retains the candidate plus `DRY_RUN_REPORT.json` under `artifacts/release-dry-run-*`.
The temporary commit is never pushed, the worktree is removed, and the evidence
is explicitly non-promotable. The same exercise runs in PR CI. It needs the full
local toolchain and read-only GitHub access. Tests cover drift, tampering, wrong
provenance/run/artifact, conflicts, interrupted uploads, idempotent retries and
post-publication failures; they simulate mutation paths without real uploads.
