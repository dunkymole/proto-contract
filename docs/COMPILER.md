# Compiler and build workflow

Run commands from the repository root unless a command uses paths from your own application. Shell blocks marked `sh` or `bash` use Bash syntax. The lock is the reviewed source of the contract version; generated modules are build outputs derived from it.

## Run the compiler

### Local installation

Use Go 1.24 or newer to build the CLI from this checkout:

```sh
go install ./cmd/proto-contract
```

Put Go's binary installation directory (`GOBIN`, or `GOPATH/bin` when unset) on `PATH`. For source-based use, replace `proto-contract` with `go run ./cmd/proto-contract` while in this repository. `snapshot`, `check`, and `update` also require `protoc` on `PATH`, or its path through `--protoc`. `generate` and `version` only read the lock and do not require `protoc`.

### Docker wrapper

The image includes the compiler and `protoc`. Build it once from this checkout:

```sh
docker build -t proto-contract .
```

In Bash, define this function so the remaining commands work unchanged:

```bash
proto-contract() {
  docker run --rm -v "$PWD:/workspace" -w /workspace proto-contract "$@"
}
```

In PowerShell, use:

```powershell
function proto-contract {
    docker run --rm -v "${PWD}:/workspace" -w /workspace proto-contract @args
}
```

Use paths inside the current directory so Docker can access them. PowerShell does not use Bash's trailing `\` for line continuation: put each command on one line, as in the examples below. A failed command sets `$LASTEXITCODE`; stop a build when it is nonzero.

## Establish and evolve a lock

Create one lock per service. `--service` is the fully qualified protobuf service name; `--api` is the stable compatibility identifier you choose. Initialize the version once when introducing the API:

```sh
proto-contract snapshot --proto proto/demo/v1/echo.proto --proto-path proto --service demo.v1.EchoService --api demo.echo --version 1.0.0 --out contracts/demo.echo.json
```

This creates parent directories and writes the output file. It refuses to replace an existing path; pass `--force` only when you deliberately want to replace it. The demo already has a lock; use it for checks rather than recreating it.

On a proposed schema change, compare it with the tracked baseline:

```sh
proto-contract check --proto proto/demo/v1/echo.proto --proto-path proto --service demo.v1.EchoService --lock contracts/demo.echo.json
```

An unchanged contract exits with code 0. A detected change exits with code 1, lists the changes, and reports the required next version. `check` does not edit the lock. After reviewing the change, update the lock and commit it with the proto:

```sh
proto-contract update --proto proto/demo/v1/echo.proto --proto-path proto --service demo.v1.EchoService --lock contracts/demo.echo.json
```

`update` applies the calculated minimum bump. `--bump minor` or `--bump major` can raise it for a behavioral change the schema cannot describe. `--bump none` is accepted only when no structural bump is required. Automatic updates leave the version unchanged when no compared fields changed. A minor bump resets patch to zero; a major bump resets minor and patch to zero. There is no automatic patch bump or `--bump patch` option.

Before merging a proposed lock, compare it with the lock and release manifest from the trusted base revision:

```sh
proto-contract release-check --base-lock /tmp/base/demo.echo.json --lock contracts/demo.echo.json --base-history /tmp/base/releases.json --history contracts/releases.json
```

The manifest records each released API, semantic version, and schema digest. It is append-only: a new version appends exactly one assignment, and an existing assignment can never point at another digest. On the initial bootstrap, when the trusted base has no manifest, the proposed history must seed the exact base-lock assignment before adding a release. A future release-integrity CI job should run this check from the pull-request base and check that the proto still matches the proposed lock.

`contracts/releases.json` is a version-controlled record, not immutable authority by itself. Protect `main` against force-pushes and direct unreviewed writes, require the lock-integrity status check, and serialize release merges with a merge queue or revalidation after each merge. The validator models one strictly increasing release line per API; after publishing a newer major, it does not accept later patches to the older major from the same history. Use a separate protected maintenance history or a distinct API identity for independently maintained lines.

## Generate during each build

1. Check the application's proto against its tracked lock; stop on failure. Do not automatically run `update` in a build, because that would accept an unreviewed change.
2. Generate ordinary protobuf messages and service bindings with the language's protobuf tools.
3. Generate Proto Contract's module from the same lock. The protoc plugin binds the same descriptor request to the lock and generates one output per explicitly mapped service.
4. Compile/package the generated module, protobuf bindings, and required runtime adapters together. Register native interceptors on matching clients/services; strict TypeScript applications create clients with `connection.client(contract)`.

For example, after a successful `check`:

```sh
proto-contract generate --lock contracts/demo.echo.json --lang python --out echo_contract.py
proto-contract generate --lock contracts/demo.echo.json --lang go --package echocontract --out gen/go/contracts/echo/contract.go
proto-contract generate --lock contracts/demo.echo.json --lang java --package protocontract.generated.echo --out src/main/java/protocontract/generated/echo/Contract.java
proto-contract generate --lock contracts/demo.echo.json --lang dotnet --package ProtoContract.Generated.Echo --out Generated/Contract.cs
protoc -I proto \
  --plugin=protoc-gen-es=node_modules/.bin/protoc-gen-es \
  --es_out=gen --es_opt=target=ts,import_extension=js \
  --plugin=protoc-gen-proto-contract=protoc-gen-proto-contract \
  --proto-contract_out=. \
  --proto-contract_opt=lang=typescript,bindings=contracts/typescript-bindings.json \
  demo/v1/echo.proto
```

The JSON bindings file contains named `service`, `lock`, and `output` properties; TypeScript entries also require `service_import` and `service_export`, and native entries may set `package`. Lock paths resolve relative to the bindings file. Outputs are relative to protoc's output root, cannot escape it, and duplicate services or output paths are rejected. The plugin validates the lock against the exact request descriptors and does not rewrite it. Native modules contain both client and server entry points; TypeScript emits a strict grpc-bridge contract definition bound to the generated Protobuf-ES service descriptor.

`generate` creates parent directories and replaces its output file. Native lock-based generation is deterministic. TypeScript generation requires `--proto`, `--service`, `--service-import`, and `--service-export`, and validates those descriptors against the lock before writing. The plugin is the recommended path when protobuf descriptors and contract bindings are generated together. Generated files must not be edited manually. If you check them in, regenerate and review them alongside the lock.

Client and server builds use their respective lock revisions. A client built against `1.0.0` stays at that version when deployed against a server built against `1.1.0`. Updating a server does not rewrite a client's contract claim.

Snapshots use lock format 2, the sole format supported by this pre-release. Format 1 and unknown format numbers are rejected by every command. The explicit lock `format` is independent of the API's semantic version: future serialization changes will be introduced deliberately with a compiler release and a documented transition policy, rather than by changing API versions or silently reinterpreting old files.

## Command options

| Command | Required flags | Other flags |
| --- | --- | --- |
| `snapshot` | `--proto`, `--service`, `--api`, `--version`, `--out` | `--proto-path` (default `.`), `--protoc` (default `protoc`), `--force` |
| `check` | `--proto`, `--service`, `--lock` | `--proto-path`, `--protoc` |
| `update` | `--proto`, `--service`, `--lock` | `--proto-path`, `--protoc`, `--bump` (default `auto`) |
| `release-check` | `--base-lock`, `--lock`, `--history` | `--base-history` (absent only for bootstrap) |
| `generate` | `--lock`, `--out` | `--lang` (default `typescript`), `--package`; TypeScript additionally requires `--proto`, `--service`, `--service-import`, `--service-export`, with optional `--proto-path` and `--protoc` |
| `version` | `--lock` | None; prints only the stored version |

The protoc plugin is invoked as `--proto-contract_opt=lang=LANG,bindings=PATH`. Supported languages are `typescript`, `go`, `python`, `java`, and `dotnet`. The bindings path is resolved from the protoc process working directory; each lock path is resolved relative to the bindings JSON file. The plugin reads only files accessible to that local process and does not fetch remote locks.

`--lang` accepts `typescript`, `python`, `go`, `java`, and `dotnet`. `--package` sets the Go/Java package or .NET namespace; Python and TypeScript reject it. Defaults and output filenames are listed in [runtime integration](RUNTIMES.md#generate-client-and-server-contracts). `--proto-path` takes one import root; paths to imported protos must resolve under it.

Use numeric `MAJOR.MINOR.PATCH` versions (no prerelease/build suffixes or leading zeros), a fully qualified protobuf service name, and a visible ASCII API identifier without spaces or `@`. Generation validates these fields and accepts lock format 2 only. API identity should remain stable as its service evolves; the lock version is separate from language package or application release versions.

## Common failures

| Symptom | Next step |
| --- | --- |
| `proto-contract` not found | Install the CLI or define the Docker wrapper above. |
| `protoc` not found / imports unresolved | Set `--protoc` or use Docker; check `--proto-path`. |
| `check` reports a required bump | Review the schema change, then deliberately run `update` and commit the lock. |
| Generated code cannot import the runtime | Include the language's runtime adapter sources/dependencies listed in the runtime guide. |
| A call returns `FAILED_PRECONDITION` | Check the API identity, the client's generated version, the server's generated version, and interceptor registration. Rebuild from the intended lock rather than overriding the header. |

The classifier's exact scope and limitations are in [compatibility policy](COMPATIBILITY.md).
