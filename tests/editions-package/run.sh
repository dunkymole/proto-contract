#!/bin/sh
set -eu
repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
fixture="$repo_root/tests/editions-package"
node_modules=${NODE_MODULES:-$repo_root/examples/clients/grpc-bridge/node_modules}
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM
cp "$fixture"/*.proto "$fixture"/*.json "$fixture"/check.ts "$tmp/"
ln -s "$node_modules" "$tmp/node_modules"
cd "$tmp"
"${PROTO_CONTRACT:-proto-contract}" snapshot --proto service.proto --proto-path . --service editions_demo.API --api editions_demo --version 1.0.0 --out contract.json
protoc -I . --descriptor_set_out=service.bin --include_imports \
  --plugin=protoc-gen-es="$node_modules/.bin/protoc-gen-es" --es_out=. --es_opt=target=ts,import_extension=js \
  --plugin=protoc-gen-proto-contract="${PROTOC_GEN_PROTO_CONTRACT:-$(command -v protoc-gen-proto-contract)}" \
  --proto-contract_out=. --proto-contract_opt=lang=typescript,bindings=bindings.json service.proto
"$node_modules/.bin/tsc" -p tsconfig.json
"$node_modules/.bin/tsx" check.ts
