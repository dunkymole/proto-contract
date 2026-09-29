#!/usr/bin/env sh
set -eu

bin=${PROTO_CONTRACT_BIN:-proto-contract}
root=${1:-.}
if [ -d "$root/rollout" ]; then
  rollout="$root/rollout"
else
  rollout="$root/examples/rollout"
fi
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

check_unchanged() {
  schema=$1
  lock=$2
  "$bin" check --proto "$rollout/proto/$schema/demo/v1/echo.proto" \
    --proto-path "$rollout/proto/$schema" \
    --service demo.v1.EchoService --lock "$rollout/contracts/$lock.lock.json"
}

expect_bump() {
  schema=$1
  old_lock=$2
  bump=$3
  next=$4
  set +e
  "$bin" check --proto "$rollout/proto/$schema/demo/v1/echo.proto" \
    --proto-path "$rollout/proto/$schema" \
    --service demo.v1.EchoService --lock "$rollout/contracts/$old_lock.lock.json" \
    --format json >"$tmp/report.json"
  status=$?
  set -e
  [ "$status" -eq 1 ] || { echo "expected descriptor drift for $schema against $old_lock" >&2; exit 1; }
  jq -e --arg bump "$bump" --arg next "$next" \
    '.status == "changed" and .bump == $bump and .next_version == $next' \
    "$tmp/report.json" >/dev/null || {
      cat "$tmp/report.json" >&2
      echo "unexpected version classification for $schema against $old_lock" >&2
      exit 1
    }
}

check_unchanged v1_0 v1_0
check_unchanged v1_1 v1_1
check_unchanged v2_0 v2_0
expect_bump v1_1 v1_0 minor 1.1.0
expect_bump v2_0 v1_1 major 2.0.0
echo "PASS rollout locks match their genuine schemas; additive and breaking schemas classify minor and major"
