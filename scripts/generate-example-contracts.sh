#!/bin/sh
set -eu

language=${1:?usage: generate-example-contracts.sh <python|go|java|dotnet>}
case "$language" in
  python|go|java|dotnet) ;;
  *) echo "unsupported example contract language: $language" >&2; exit 2 ;;
esac

bindings="contracts/native-bindings.${language}.json"
if [ "$language" = dotnet ] && [ "${2:-server}" = client ]; then
  bindings="contracts/native-bindings.dotnet-client.json"
fi

protoc -I proto \
  --plugin=protoc-gen-proto-contract=/protoc-gen-proto-contract \
  --proto-contract_out=. \
  --proto-contract_opt="lang=${language},bindings=${bindings}" \
  demo/v1/echo.proto
