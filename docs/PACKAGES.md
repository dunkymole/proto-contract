# Runtime packages — v0.5.0

Runtime packages use [v0.5.0 GitHub Release assets](https://github.com/dunkymole/proto-contract/releases/tag/v0.5.0) after publication. During preparation, download the candidate artifact from the [read-only workflow](https://github.com/dunkymole/proto-contract/actions/workflows/package-release.yml); preparing it creates no public tag or Release. Python, Java and .NET packages are distributed as files, without uploads to PyPI, Maven Central or NuGet.org. Normal Go installation requires the public repository tag. Package version `0.5.0` versions the adapter
implementation, independently of the API version in each generated contract lock.

| Language | Coordinate | Build/test baseline | Dependency policy |
| --- | --- | --- | --- |
| Go | `github.com/dunkymole/proto-contract/runtimes/go/protocontract` | Go 1.27.2; module requires Go 1.26.9+ | gRPC-Go 1.84.0, x/net 0.61.0; normal Go minimal version selection |
| Python | `proto-contract-runtime` (import `proto_contract`) | Python 3.13; requires Python 3.10+ | grpcio >=1.71,<2 |
| Java | `io.github.dunkymole:proto-contract-runtime` | JDK 21 build, Java 17 bytecode | grpc-api 1.71.0; Maven dependency mediation |
| .NET | `ProtoContract.Runtime` (namespace `ProtoContract`) | .NET 9 SDK; targets net8.0 | Grpc.Core.Api >=2.71,<3 |
| .NET server integration | `ProtoContract.AspNetCore` (namespace `ProtoContract`) | .NET 9 SDK; targets net8.0 | Runtime package, ASP.NET Core, Grpc.AspNetCore.Server >=2.71,<3 |

The minimum language versions are package requirements, not a claim that every
language/dependency combination has been tested. The matrix uses the baseline
versions above. Compiler and adapter artifacts should initially use the same
release version; any future independent releases must retain generated public
API compatibility or document the required compiler upgrade.

The Go minimum was raised for patched dependencies and standard-library fixes.
Go 1.22–1.25 consumers must upgrade their toolchain before using v0.5.0. CI and
release preparation run govulncheck v1.9.0 against the current vulnerability
database and fail for reachable findings. A clean scan covers the analyzed Go
source graph; it is not a security audit of every language package or deployment.

## Install v0.5.0

Download the packages, `manifest.json` and `SHA256SUMS` from the candidate Actions
artifact, or from the [v0.5.0 release](https://github.com/dunkymole/proto-contract/releases/tag/v0.5.0) after publication.
From the download directory, verify `sha256sum --check SHA256SUMS`, then install
the packages you need:

```sh
python -m pip install ./proto_contract_runtime-0.5.0-py3-none-any.whl
mvn install:install-file -Dfile=proto-contract-runtime-0.5.0.jar -DpomFile=proto-contract-runtime-0.5.0.pom
dotnet nuget add source /absolute/path/to/downloads --name proto-contract-release
dotnet add package ProtoContract.Runtime --version 0.5.0
# ASP.NET Core servers also install:
dotnet add package ProtoContract.AspNetCore --version 0.5.0
go get github.com/dunkymole/proto-contract@v0.5.0
```

The file-based Python, Java and .NET commands also work with candidate downloads.
The normal `go get` command requires publication of the tag; use the isolated Go
test-proxy instructions below to test an unpublished candidate.
Keep NuGet.org enabled for transitive dependencies. Maven installs the downloaded
JAR and POM into your local repository. Go uses the public repository tag and
normal module checksum verification; do not set `GONOSUMDB` for published use.
The Go file-proxy ZIP is only a local testing convenience, and its contents and
module checksum can differ from the canonical public module ZIP.

The release manifest records the exact clean source commit and package hashes.
Release preparation validates clean consumers and checksums before publication.
The release notes link the validation run for the published source revision.

## Build and verify without publication

From the repository root, with Docker Desktop (Linux containers) and PowerShell:

```powershell
./scripts/test-packages.ps1
```

This builds a Python wheel and source distribution, Java binary, source and API-documentation JARs and
POM, core and ASP.NET Core NuGet packages, and a Go file-proxy module ZIP under `artifacts/packages/`.
`manifest.json` records SHA-256 checksums, the source revision and whether the
source tree had uncommitted changes when built.
It runs the starter in a fresh virtual environment, then the 20-combination
interoperability matrix. Native client/server containers install the packages;
they do not copy adapter sources into the consumer project. Existing matrix
checks cover all four RPC shapes, invalid metadata, service isolation and strict
dispatch. The Python server also runs its generated-server unit tests.

The Go artifact uses the existing root module, preserving generated imports.
The public module uses the matching release tag. Earlier releases and tags remain unchanged. The proxy archive contains
the compiler, runtime and Go test fixtures. Consumer-generated bindings belong to
the application's own module; the matrix uses a separate `starter.local/demo`
module and rewrites demo imports to that consumer module.

## Clean starter

[`examples/starter/python`](../examples/starter/python/) is a small consumer:
generated protobuf bindings, generated contract binding, and one application
file. Its Dockerfile installs the built wheel into a new virtual environment.
The generated client succeeds. Missing metadata and an incompatible major version
fail with `FAILED_PRECONDITION`, with a handler counter proving neither rejection
entered application logic.

To install the local artifacts in your own project:

```sh
python -m pip install artifacts/packages/python/*.whl
mvn install:install-file -Dfile=artifacts/packages/java/proto-contract-runtime-0.5.0.jar -DpomFile=artifacts/packages/java/pom.xml
dotnet nuget add source /absolute/path/to/artifacts/packages/dotnet --name proto-contract-local
dotnet add package ProtoContract.Runtime --version 0.5.0
# ASP.NET Core servers also install:
dotnet add package ProtoContract.AspNetCore --version 0.5.0
```

Keep NuGet.org enabled for the transitive gRPC dependency. The container examples
use `packaging/NuGet.Config` to map the runtime specifically to the local feed.

For Go, use a fresh `GOMODCACHE`, set `GOPROXY=file:///absolute/path/to/artifacts/packages/go,https://proxy.golang.org`
and `GONOSUMDB=github.com/dunkymole/proto-contract` in an isolated test environment,
then `go get github.com/dunkymole/proto-contract@v0.5.0`. This bypass is only for
the unpublished local artifact; normal published consumption must use checksums.

Generate each service binding from its build's checked lock, then follow
[runtime wiring](RUNTIMES.md). Install strict dispatch globally, cover every
registered service, and explicitly exempt infrastructure services when needed.
The public API includes inferred typed client/registration helpers, server builders,
ASP.NET Core extensions, and the existing low-level interceptors, strict dispatch
and coverage helpers. Runtime internals are not a separate
stability promise. Python's asynchronous grpc.aio API is not covered by this
synchronous adapter.

## Release process

Follow the [package release runbook](RELEASING.md) for checked version updates,
read-only candidate preparation, explicit publication and verification. The
current version and managed references are checked by `scripts/release.py check`.
The existing build/test scripts above remain the package-validation entry points.

Release assets include the packages, manifest and checksums; workflow candidates
also carry source/run metadata and reviewable release notes. Publication promotes
those exact tested bytes and is a separate explicit command. The public Go tag is
itself publication. TypeScript distribution remains coordinated with grpc-bridge;
C++ is outside this package process. Application API contract versions and their
release history remain independent of compiler/runtime package releases.
