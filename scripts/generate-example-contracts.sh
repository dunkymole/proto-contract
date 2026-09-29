#!/bin/sh
set -eu

language=${1:?usage: generate-example-contracts.sh <python|go|java|dotnet>}
case "$language" in
  python|go|java|dotnet) ;;
  *) echo "unsupported example contract language: $language" >&2; exit 2 ;;
esac

protoc -I proto \
  --plugin=protoc-gen-proto-contract=/protoc-gen-proto-contract \
  --proto-contract_out=. \
  --proto-contract_opt="lang=${language},bindings=contracts/native-bindings.${language}.json" \
  demo/v1/echo.proto
