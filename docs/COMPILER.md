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

This creates parent directories and writes the output file, replacing it if present. The demo already has a lock; use it for checks rather than recreating it.

On a proposed schema change, compare it with the tracked baseline:

```sh
proto-contract check --proto proto/demo/v1/echo.proto --proto-path proto --service demo.v1.EchoService --lock contracts/demo.echo.json
```

An unchanged contract exits with code 0. A detected change exits with code 1, lists the changes, and reports the required next version. `check` does not edit the lock. After reviewing the change, update the lock and commit it with the proto:

```sh
proto-contract update --proto proto/demo/v1/echo.proto --proto-path proto --service demo.v1.EchoService --lock contracts/demo.echo.json
```

`update` applies the calculated minimum bump. `--bump minor` or `--bump major` can raise it for a behavioral change the schema cannot describe. `--bump none` is accepted only when no structural bump is required. Automatic updates leave the version unchanged when no compared fields changed. A minor bump resets patch to zero; a major bump resets minor and patch to zero. There is no automatic patch bump or `--bump patch` option.

## Generate during each build

1. Check the application's proto against its tracked lock; stop on failure. Do not automatically run `update` in a build, because that would accept an unreviewed change.
2. Generate ordinary protobuf messages and service bindings with the language's protobuf tools.
3. Generate Proto Contract's module from the same lock.
4. Compile/package the generated module, protobuf bindings, and required runtime adapters together. Register the generated interceptor on each client and matching service.

For example, after a successful `check`:

```sh
proto-contract generate --lock contracts/demo.echo.json --lang python --out echo_contract.py
proto-contract generate --lock contracts/demo.echo.json --lang go --package echocontract --out gen/go/contracts/echo/contract.go
proto-contract generate --lock contracts/demo.echo.json --lang java --package protocontract.generated.echo --out src/main/java/protocontract/generated/echo/Contract.java
proto-contract generate --lock contracts/demo.echo.json --lang dotnet --package ProtoContract.Generated.Echo --out Generated/Contract.cs
proto-contract generate --lock contracts/demo.echo.json --lang typescript --out src/gen/echo_contract.ts
```

Choose the command for your language. Native modules contain both client and server entry points; TypeScript supplies the grpc-bridge client interceptor. See [runtime integration](RUNTIMES.md) for registration and dependencies.

`generate` creates parent directories and replaces its output file. It is deterministic for the same lock and options. It does not verify the proto, modify the lock, calculate a version, or generate protobuf bindings. Generated files must not be edited manually. If you check them in, regenerate and review them alongside the lock; the Go demo contract is checked in and tested for consistency. Other demo contract modules are generated inside Docker images.

Client and server builds use their respective lock revisions. A client built against `1.0.0` stays at that version when deployed against a server built against `1.1.0`. Updating a server does not rewrite a client's contract claim.

Snapshots use lock format 2, the sole format supported by this pre-release. Format 1 and unknown format numbers are rejected by every command. The explicit lock `format` is independent of the API's semantic version: future serialization changes will be introduced deliberately with a compiler release and a documented transition policy, rather than by changing API versions or silently reinterpreting old files.

## Command options

| Command | Required flags | Other flags |
| --- | --- | --- |
| `snapshot` | `--proto`, `--service`, `--api`, `--version`, `--out` | `--proto-path` (default `.`), `--protoc` (default `protoc`) |
| `check` | `--proto`, `--service`, `--lock` | `--proto-path`, `--protoc` |
| `update` | `--proto`, `--service`, `--lock` | `--proto-path`, `--protoc`, `--bump` (default `auto`) |
| `generate` | `--lock`, `--out` | `--lang` (default `typescript`), `--package` |
| `version` | `--lock` | None; prints only the stored version |

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
