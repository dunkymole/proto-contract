#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
go build -o "$work/proto-contract" ./cmd/proto-contract
go build -o "$work/protoc-gen-proto-contract" ./cmd/protoc-gen-proto-contract
cp internal/contract/testdata/editions/service.proto "$work/"
schema=(--proto service.proto --proto-path "$work" --service editions_demo.API)
"$work/proto-contract" snapshot "${schema[@]}" --api editions_demo --version 1.0.0 --out "$work/lock.json"
"$work/proto-contract" check "${schema[@]}" --lock "$work/lock.json"
for lang in python go java dotnet; do
 "$work/proto-contract" generate --proto service.proto --proto-path "$work" --lock "$work/lock.json" --lang "$lang" --out "$work/contract.$lang"
done
cat > "$work/bindings.json" <<'JSON'
{"bindings":[{"service":"editions_demo.API","lock":"lock.json","output":"plugin.py"}]}
JSON
protoc -I "$work" --plugin="protoc-gen-proto-contract=$work/protoc-gen-proto-contract" --proto-contract_out="$work" --proto-contract_opt="lang=python,bindings=$work/bindings.json" service.proto
cmp "$work/contract.python" "$work/plugin.py"
sed -i 's/EXPANDED/PACKED/' "$work/service.proto"
if "$work/proto-contract" check "${schema[@]}" --lock "$work/lock.json"; then
 echo 'check accepted changed edition semantics' >&2; exit 1
fi
if "$work/proto-contract" update "${schema[@]}" --lock "$work/lock.json" --bump minor; then
 echo 'update accepted a minor edition feature change' >&2; exit 1
fi
"$work/proto-contract" update "${schema[@]}" --lock "$work/lock.json"
test "$("$work/proto-contract" version --lock "$work/lock.json")" = 2.0.0
echo 'Edition CLI, plugin and major-bump checks passed'
