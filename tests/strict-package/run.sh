#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
fixture="$repo_root/tests/strict-package"
node_modules=${NODE_MODULES:-$repo_root/node_modules}
if [ ! -x "$node_modules/.bin/tsc" ]; then
  node_modules="$repo_root/examples/clients/grpc-bridge/node_modules"
fi
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM
cp "$fixture"/*.proto "$fixture"/bindings.json "$fixture"/check.ts "$fixture"/tsconfig.json "$fixture"/package.json "$tmp/"
ln -s "$node_modules" "$tmp/node_modules"
cd "$tmp"

"${PROTO_CONTRACT:-proto-contract}" snapshot \
  --proto manager.proto --proto-path . \
  --service review.binding.ContractService --api review.binding \
  --version 1.0.0 --out contract.json
protoc -I . --descriptor_set_out=manager.bin --include_imports \
  --plugin=protoc-gen-es="${PROTOC_GEN_ES:-$node_modules/.bin/protoc-gen-es}" \
  --es_out=. --es_opt=target=ts,import_extension=js \
  --plugin=protoc-gen-proto-contract="${PROTOC_GEN_PROTO_CONTRACT:-$(command -v protoc-gen-proto-contract)}" \
  --proto-contract_out=. \
  --proto-contract_opt=lang=typescript,bindings=bindings.json manager.proto imported.proto
"$node_modules/.bin/tsc" -p tsconfig.json
"$node_modules/.bin/tsx" check.ts
