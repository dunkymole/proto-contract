# @dunkymole/proto-contract

The Proto Contract compiler and protoc plugin, packaged for Node.js projects. Requires Node.js 22 or newer. Includes both native commands for Linux, macOS, and Windows on x64 and arm64; no Go installation, lifecycle scripts, or install-time binary downloads are needed.

```sh
npm install --save-dev @dunkymole/proto-contract@0.6.0
npx proto-contract snapshot --proto service.proto --proto-path . --service example.API --api example --version 1.0.0 --out contract.json
npx proto-contract check --proto service.proto --proto-path . --service example.API --lock contract.json
```

Install `protoc` separately and put it on your PATH. Protobuf Edition 2023 inputs require a compiler with Edition 2023 support. This package contains the contract compiler and plugin; it does not include protobuf language generators or language runtime packages.

## Use as a protoc plugin

Inside an npm script, npm adds `node_modules/.bin` to PATH, so protoc can discover `protoc-gen-proto-contract` directly:

```json
{
  "scripts": {
    "generate:contracts": "protoc -I . --proto-contract_out=. --proto-contract_opt=lang=python,bindings=bindings.json service.proto"
  }
}
```

The bindings file associates a service with its checked lock and output:

```json
{"bindings":[{"service":"example.API","lock":"contract.json","output":"service_contract.py"}]}
```

For an explicit Windows `protoc --plugin` path, point to the bundled `.exe`, rather than npm's `.cmd` shim. The path can be resolved without launching a process:

```js
const { resolveBinary } = require('@dunkymole/proto-contract/bin/launcher.cjs');
console.log(resolveBinary('protoc-gen-proto-contract'));
```

Commands preserve arguments, standard input/output/error, exit status, and native process failures. Unsupported OS/architecture combinations fail with a diagnostic rather than downloading or substituting another executable.

`native-manifest.json` records the source revision, Go/Node/npm toolchain versions, and SHA-256 digest of every native executable. `THIRD_PARTY_NOTICES.md` includes the Go standard library license and licenses for external modules actually linked into these commands. Reproducible builds use the checked source and Go module versions; the builder disables CGO, source paths, VCS build metadata, and variable build IDs.

See the [compiler guide](https://github.com/dunkymole/proto-contract/blob/main/docs/COMPILER.md) and [compatibility policy](https://github.com/dunkymole/proto-contract/blob/main/docs/COMPATIBILITY.md) for the supported descriptor surface and required version changes.
