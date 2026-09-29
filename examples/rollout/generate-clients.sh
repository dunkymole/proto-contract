#!/bin/sh
set -eu

for version in v1_0 v1_1 v2_0; do
  mkdir -p "rollout/gen/$version"
  protoc -I "rollout/proto/$version" \
    --plugin=protoc-gen-es=./node_modules/.bin/protoc-gen-es \
    --es_out="rollout/gen/$version" --es_opt=target=ts,import_extension=js \
    --plugin=protoc-gen-proto-contract=/usr/local/bin/protoc-gen-proto-contract \
    --proto-contract_out=. \
    --proto-contract_opt="lang=typescript,bindings=rollout/contracts/${version}.bindings.json" \
    demo/v1/echo.proto
done
