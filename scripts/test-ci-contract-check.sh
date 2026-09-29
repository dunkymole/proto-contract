#!/usr/bin/env bash
set -euo pipefail

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM
export PROTO_CONTRACT_BIN=${PROTO_CONTRACT_BIN:-proto-contract}
export GITHUB_OUTPUT="$tmp/output"
export GITHUB_STEP_SUMMARY="$tmp/summary"

mkdir -p "$tmp/proto"
cat >"$tmp/proto/echo.proto" <<'EOF'
syntax = "proto3";
package ci.v1;
message Request { string text = 1; }
message Response { string text = 1; }
service Echo { rpc Call(Request) returns (Response); }
EOF

"$PROTO_CONTRACT_BIN" snapshot --proto "$tmp/proto/echo.proto" --proto-path "$tmp/proto" \
  --service ci.v1.Echo --api ci.echo --version 1.0.0 --out "$tmp/base.lock.json" >/dev/null

run_helper() {
  bash "$repo_root/scripts/ci-contract-check.sh" --proto "$tmp/proto/echo.proto" \
    --proto-path "$tmp/proto" --service ci.v1.Echo --lock "$tmp/base.lock.json" "$@"
}

# Unchanged is successful and exposes the parsed fields to downstream steps.
run_helper
grep -Fx 'api=ci.echo' "$GITHUB_OUTPUT" >/dev/null
grep -Fx 'version=1.0.0' "$GITHUB_OUTPUT" >/dev/null
grep -Fx 'next_version=1.0.0' "$GITHUB_OUTPUT" >/dev/null
grep -Fx 'bump=none' "$GITHUB_OUTPUT" >/dev/null
grep -Fx 'changed=false' "$GITHUB_OUTPUT" >/dev/null
grep -F 'No descriptor changes.' "$GITHUB_STEP_SUMMARY" >/dev/null

# A genuine additive descriptor change keeps its JSON report and fails the job.
cat >>"$tmp/proto/echo.proto" <<'EOF'
// New response field is compatible and requires a minor bump.
EOF
sed -i 's/message Response { string text = 1; }/message Response { string text = 1; string detail = 2; }/' "$tmp/proto/echo.proto"
set +e
run_helper >"$tmp/changed.stdout" 2>"$tmp/changed.stderr"
changed_status=$?
set -e
[ "$changed_status" -eq 1 ] || { cat "$tmp/changed.stderr" >&2; echo "changed contract did not fail" >&2; exit 1; }
grep -Fx 'next_version=1.1.0' "$GITHUB_OUTPUT" >/dev/null
grep -Fx 'bump=minor' "$GITHUB_OUTPUT" >/dev/null
grep -Fx 'changed=true' "$GITHUB_OUTPUT" >/dev/null
grep -F 'field added:' "$GITHUB_STEP_SUMMARY" >/dev/null

# Malformed source must not produce a success report or publish outputs.
: >"$GITHUB_OUTPUT"
echo 'syntax = "proto3"; package broken; this is not protobuf' >"$tmp/proto/echo.proto"
set +e
run_helper >"$tmp/malformed.stdout" 2>"$tmp/malformed.stderr"
malformed_status=$?
set -e
[ "$malformed_status" -ne 0 ] || { echo "malformed descriptor unexpectedly succeeded" >&2; exit 1; }
[ ! -s "$GITHUB_OUTPUT" ] || { echo "malformed input published CI outputs" >&2; exit 1; }
[ ! -s "$tmp/malformed.stdout" ] || { echo "malformed input published a JSON success report" >&2; exit 1; }

# A major increment at the semver component limit must fail without an empty
# or otherwise misleading next_version in the report.
jq '.version = "2147483647.0.0"' "$tmp/base.lock.json" >"$tmp/max-version.lock.json"
cat >"$tmp/proto/echo.proto" <<'EOF'
syntax = "proto3";
package ci.v1;
message Request { int32 text = 1; }
message Response { string text = 1; }
service Echo { rpc Call(Request) returns (Response); }
EOF
set +e
"$PROTO_CONTRACT_BIN" check --proto "$tmp/proto/echo.proto" --proto-path "$tmp/proto" \
  --service ci.v1.Echo --lock "$tmp/max-version.lock.json" --format json \
  >"$tmp/overflow.stdout" 2>"$tmp/overflow.stderr"
overflow_status=$?
set -e
[ "$overflow_status" -ne 0 ] || { echo "overflowing version increment unexpectedly succeeded" >&2; exit 1; }
[ ! -s "$tmp/overflow.stdout" ] || { echo "overflowing increment published a misleading report" >&2; exit 1; }

# A release-history failure is propagated even when descriptor comparison succeeds.
cat >"$tmp/proto/echo.proto" <<'EOF'
syntax = "proto3";
package ci.v1;
message Request { string text = 1; }
message Response { string text = 1; }
service Echo { rpc Call(Request) returns (Response); }
EOF
printf '{"format":1,"releases":[]}' >"$tmp/history.json"
set +e
run_helper --base-lock "$tmp/base.lock.json" --history "$tmp/history.json" >"$tmp/history.stdout" 2>"$tmp/history.stderr"
history_status=$?
set -e
[ "$history_status" -ne 0 ] || { echo "invalid release history unexpectedly succeeded" >&2; exit 1; }
grep -F 'Release history validation failed.' "$GITHUB_STEP_SUMMARY" >/dev/null

echo "PASS CI check helper: unchanged outputs, changed report/failure, malformed input, version overflow, release-history failure"
