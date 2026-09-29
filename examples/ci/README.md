# Contract checks in CI

`proto-contract check --format json` prints a machine-readable report to stdout. It
returns exit status 1 when the descriptor differs from the checked-in lock, while
still emitting the report so CI can publish the required version bump and change
list. Invalid inputs do not produce a report.

The `scripts/ci-contract-check.sh` helper exposes `api`, `version`,
`next_version`, `bump`, and `changed` through GitHub Actions' `$GITHUB_OUTPUT` and
writes a Markdown summary through `$GITHUB_STEP_SUMMARY`. It exits unsuccessfully
for a changed contract or failed release-history validation. Pass `--base-lock`,
`--history`, and optionally `--base-history` together to run
`proto-contract release-check` against trusted base history in the same job.

Example step after installing `proto-contract`, `protoc`, and `jq`:

```yaml
- uses: actions/checkout@v4
  with:
    ref: ${{ github.event.pull_request.base.sha }}
    path: trusted-base
    persist-credentials: false
- name: Check API contract
  id: contract
  working-directory: ${{ github.workspace }}
  run: |
    scripts/ci-contract-check.sh \
      --proto api/echo.proto \
      --proto-path api \
      --service demo.v1.EchoService \
      --lock contracts/echo.json \
      --base-lock trusted-base/contracts/echo.json \
      --base-history trusted-base/contracts/releases.json \
      --history contracts/releases.json
- name: Report proposed contract version
  if: always() && steps.contract.outputs.api != ''
  run: echo "${{ steps.contract.outputs.api }} needs ${{ steps.contract.outputs.bump }} bump to ${{ steps.contract.outputs.next_version }}"
```

The base lock and manifest must come from the trusted target revision, not from
the proposed checkout. The example checks an existing API; for a new API,
establish its reviewed baseline and history assignment before enabling the
release-transition check. A changed contract should fail the check until its
lock and append-only release history are updated; the JSON output and job
summary remain available to explain the failure. This snippet assumes the
workflow has already checked out the proposed commit into the workspace where
the helper command runs; only the trusted base files are read from the separate
`trusted-base` checkout.
