#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "usage: ci-contract-check.sh --proto FILE --proto-path DIR --service NAME --lock FILE [--base-lock FILE --history FILE [--base-history FILE]]" >&2
  exit 2
}

proto= proto_path=. service= lock= base_lock= base_history= history=
while (($#)); do
  case "$1" in
    --proto) (($# >= 2)) || usage; proto=$2; shift 2 ;;
    --proto-path) (($# >= 2)) || usage; proto_path=$2; shift 2 ;;
    --service) (($# >= 2)) || usage; service=$2; shift 2 ;;
    --lock) (($# >= 2)) || usage; lock=$2; shift 2 ;;
    --base-lock) (($# >= 2)) || usage; base_lock=$2; shift 2 ;;
    --base-history) (($# >= 2)) || usage; base_history=$2; shift 2 ;;
    --history) (($# >= 2)) || usage; history=$2; shift 2 ;;
    *) usage ;;
  esac
done
[[ -n $proto && -n $service && -n $lock ]] || usage
if [[ -n $base_lock || -n $base_history || -n $history ]]; then
  [[ -n $base_lock && -n $history ]] || usage
fi

bin=${PROTO_CONTRACT_BIN:-proto-contract}
stderr_file=$(mktemp)
trap 'rm -f "$stderr_file"' EXIT
set +e
report=$("$bin" check --proto "$proto" --proto-path "$proto_path" --service "$service" --lock "$lock" --format json 2>"$stderr_file")
check_status=$?
set -e
if ! jq -e 'type == "object" and (.api|type=="string") and (.status=="changed" or .status=="unchanged") and (.version|type=="string") and (.next_version|type=="string") and (.bump|type=="string") and (.changes|type=="array")' >/dev/null <<<"$report"; then
  cat "$stderr_file" >&2 || true
  echo "proto-contract check did not produce a valid JSON report" >&2
  exit 2
fi

api=$(jq -r .api <<<"$report")
version=$(jq -r .version <<<"$report")
next_version=$(jq -r .next_version <<<"$report")
bump=$(jq -r .bump <<<"$report")
status=$(jq -r .status <<<"$report")
changed=false
[[ $status == changed ]] && changed=true

if [[ -n ${GITHUB_OUTPUT:-} ]]; then
  {
    printf 'api=%s\nversion=%s\nnext_version=%s\nbump=%s\nchanged=%s\n' "$api" "$version" "$next_version" "$bump" "$changed"
  } >>"$GITHUB_OUTPUT"
fi
if [[ -n ${GITHUB_STEP_SUMMARY:-} ]]; then
  {
    printf '### Contract check: `%s`\n\n' "$api"
    printf '| Current | Proposed | Minimum bump | Result |\n|---|---|---|---|\n| `%s` | `%s` | `%s` | %s |\n\n' "$version" "$next_version" "$bump" "$status"
    jq -r 'if (.changes|length)==0 then "No descriptor changes." else (.changes[] | "- " + .) end' <<<"$report"
  } >>"$GITHUB_STEP_SUMMARY"
fi

release_status=0
if [[ -n $base_lock ]]; then
  release_args=(release-check --base-lock "$base_lock" --lock "$lock" --history "$history")
  [[ -z $base_history ]] || release_args+=(--base-history "$base_history")
  if ! "$bin" "${release_args[@]}"; then
    release_status=1
    if [[ -n ${GITHUB_STEP_SUMMARY:-} ]]; then
      printf '\nRelease history validation failed.\n' >>"$GITHUB_STEP_SUMMARY"
    fi
  fi
fi

if ((check_status != 0)); then
  cat "$stderr_file" >&2 || true
fi
if ((check_status != 0 || release_status != 0)); then
  exit 1
fi
