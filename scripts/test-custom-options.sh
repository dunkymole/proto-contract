#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
go build -o "$work/proto-contract" ./cmd/proto-contract
go build -o "$work/protoc-gen-proto-contract" ./cmd/protoc-gen-proto-contract
cp internal/contract/testdata/options/*.proto "$work/"

# Exercise source-only annotations when the installed protoc supports retention.
help=$(protoc --help)
if [[ "$help" == *--retain_options* ]]; then
  sed -i 's/optional Rules rules = 50001;/optional Rules rules = 50001 [retention = RETENTION_SOURCE];/' "$work/policy.proto"
fi

schema=(--proto service.proto --proto-path "$work" --service annotated.API)
"$work/proto-contract" snapshot "${schema[@]}" --api annotated --version 1.0.0 --out "$work/lock.json"
"$work/proto-contract" check "${schema[@]}" --lock "$work/lock.json"
for lang in python go java dotnet; do
  "$work/proto-contract" generate --proto service.proto --proto-path "$work" --lock "$work/lock.json" --lang "$lang" --out "$work/contract.$lang"
done

cat > "$work/bindings.json" <<'JSON'
{"bindings":[{"service":"annotated.API","lock":"lock.json","output":"plugin.py"}]}
JSON
protoc -I "$work" --plugin="protoc-gen-proto-contract=$work/protoc-gen-proto-contract" \
  --proto-contract_out="$work" --proto-contract_opt="lang=python,bindings=$work/bindings.json" service.proto
cmp "$work/contract.python" "$work/plugin.py"

# The contract must change even though protobuf wire shape remains identical.
sed -i 's/min_length: 1/min_length: 2/' "$work/service.proto"
if "$work/proto-contract" check "${schema[@]}" --lock "$work/lock.json"; then
  echo 'check accepted a changed validation annotation' >&2
  exit 1
fi
if "$work/proto-contract" update "${schema[@]}" --lock "$work/lock.json" --bump minor; then
  echo 'update accepted a minor bump for a validation annotation change' >&2
  exit 1
fi
"$work/proto-contract" update "${schema[@]}" --lock "$work/lock.json"
test "$("$work/proto-contract" version --lock "$work/lock.json")" = 2.0.0
"$work/proto-contract" check "${schema[@]}" --lock "$work/lock.json"
echo 'Custom-option CLI, plugin and major-bump checks passed'
