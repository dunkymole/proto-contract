# Runtime packages — v0.3.0

Runtime packages are published as [v0.3.0 GitHub Release assets](https://github.com/dunkymole/proto-contract/releases/tag/v0.3.0). Python, Java and .NET packages are downloaded from that release; they are not published to PyPI, Maven Central or NuGet.org. Go uses the public repository tag. Package version `0.3.0` versions the adapter
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
Go 1.22–1.25 consumers must upgrade their toolchain before using v0.3.0. CI and
release preparation run govulncheck v1.9.0 against the current vulnerability
database and fail for reachable findings. A clean scan covers the analyzed Go
source graph; it is not a security audit of every language package or deployment.

## Install v0.3.0

Download the packages, `manifest.json` and `SHA256SUMS` from the
[v0.3.0 release](https://github.com/dunkymole/proto-contract/releases/tag/v0.3.0).
From the download directory, verify `sha256sum --check SHA256SUMS`, then install
the packages you need:

```sh
python -m pip install ./proto_contract_runtime-0.3.0-py3-none-any.whl
mvn install:install-file -Dfile=proto-contract-runtime-0.3.0.jar -DpomFile=proto-contract-runtime-0.3.0.pom
dotnet nuget add source /absolute/path/to/downloads --name proto-contract-release
dotnet add package ProtoContract.Runtime --version 0.3.0
# ASP.NET Core servers also install:
dotnet add package ProtoContract.AspNetCore --version 0.3.0
go get github.com/dunkymole/proto-contract@v0.3.0
```

Keep NuGet.org enabled for transitive dependencies. Maven installs the downloaded
JAR and POM into your local repository. Go uses the public repository tag and
normal module checksum verification; do not set `GONOSUMDB` for published use.
The Go file-proxy ZIP is only a local testing convenience, and its contents and
module checksum can differ from the canonical public module ZIP.

The release was built from clean commit
`6108813bfedb46e7cf92a1178e915e2c2eadb28b` by
[the package validation workflow](https://github.com/dunkymole/proto-contract/actions/runs/39204971566).
Clean-consumer tests passed, the published assets were downloaded and their
checksums verified, and the Go tag was fetched through the public module proxy
with checksum verification enabled.

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
The published `v0.3.0` tag resolves to commit `6108813bfedb46e7cf92a1178e915e2c2eadb28b`. Earlier releases and tags remain unchanged. The proxy archive contains
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
mvn install:install-file -Dfile=artifacts/packages/java/proto-contract-runtime-0.3.0.jar -DpomFile=artifacts/packages/java/pom.xml
dotnet nuget add source /absolute/path/to/artifacts/packages/dotnet --name proto-contract-local
dotnet add package ProtoContract.Runtime --version 0.3.0
# ASP.NET Core servers also install:
dotnet add package ProtoContract.AspNetCore --version 0.3.0
```

Keep NuGet.org enabled for the transitive gRPC dependency. The container examples
use `packaging/NuGet.Config` to map the runtime specifically to the local feed.

For Go, use a fresh `GOMODCACHE`, set `GOPROXY=file:///absolute/path/to/artifacts/packages/go,https://proxy.golang.org`
and `GONOSUMDB=github.com/dunkymole/proto-contract` in an isolated test environment,
then `go get github.com/dunkymole/proto-contract@v0.3.0`. This bypass is only for
the unpublished local artifact; normal published consumption must use checksums.

Generate each service binding from its build's checked lock, then follow
[runtime wiring](RUNTIMES.md). Install strict dispatch globally, cover every
registered service, and explicitly exempt infrastructure services when needed.
The public API includes inferred typed client/registration helpers, server builders,
ASP.NET Core extensions, and the existing low-level interceptors, strict dispatch
and coverage helpers. Runtime internals are not a separate
stability promise. Python's asynchronous grpc.aio API is not covered by this
synchronous adapter.

## Release preparation

First test compiler and adapters together, then record their versions and the
tested grpc-bridge revision (currently pinned by the CI workflow). Verify artifacts
from a clean consumer and review dependency/license metadata before authorizing
any upload. Python and Java artifacts include MIT notices; NuGet includes the MIT
expression and license file; the Go archive includes the root license. Java ZIP
timestamps and Go ZIP entries are fixed; Python uses SOURCE_DATE_EPOCH and .NET
uses deterministic compilation. Cross-machine byte-for-byte reproducibility is
not yet asserted for every ecosystem.

There are no upload or deploy steps in the local package scripts. TypeScript remains
coordinated with grpc-bridge's distribution; C++ is outside this package work.

## GitHub Releases preparation

`.github/workflows/package-release.yml` runs manually through **Actions → Prepare
package release → Run workflow**. Its default `create_draft=false` builds and tests
all packages, runs the clean starter and interoperability matrix, prepares release
assets, checks their SHA-256 hashes, and stores a downloadable Actions artifact
for 30 days. It requires only repository read access and creates no Release or tag.
The workflow must first be committed and made available on the default branch
before GitHub offers its manual trigger. Merging it does not run the workflow.

To prepare the assets locally after `scripts/test-packages.ps1`:

```sh
python scripts/prepare-release.py
cd artifacts/release
sha256sum --check SHA256SUMS
```

On Windows without Python installed, run the helper using the existing container:

```powershell
docker run --rm -v "${PWD}:/workspace" -w /workspace python:3.13-slim python scripts/prepare-release.py
```

The output includes the Python wheel/source distribution, Java binary/source/API
documentation JARs and a versioned POM, NuGet package, Go file-proxy ZIP,
`manifest.json`, and `SHA256SUMS`. The helper verifies every input against the build
manifest and refuses missing, extra, modified or inconsistently versioned packages.
It also refuses an existing output directory: use `--out` with a fresh destination
for subsequent preparations. Checksums detect corrupted downloads; they are not
cryptographic signatures or an independent proof of publisher identity.

The published package version is **v0.3.0**. Its release includes the packages, clean-source manifest and `SHA256SUMS`.
This minor release includes inferred native bindings, typed client factories,
combined server registration and the ASP.NET Core integration package. Upgrade
compiler and runtime packages together when regenerating these helpers; earlier
runtimes do not provide their new builder APIs. The RPC metadata format and API
contract compatibility policy are unchanged. This package bump does not require
bumping application contract locks.
For subsequent versions, update the Python/Java/.NET package metadata, Go proxy
builder, consumer dependency versions, build manifest version and package
verification fixtures together. Commit the tested source and create its matching
`vNEXT` tag only when release work is authorized (substitute the new package version). Run this workflow on that
exact tag with `create_draft=true`. The draft job alone receives `contents: write`;
it checks the tag points to the workflow commit, verifies clean-source provenance
and downloaded checksums, then creates an **unpublished draft** with the assets.
An existing release causes creation to fail; assets are never overwritten.

Review the draft and its assets before publishing manually in GitHub. No workflow
step publishes the draft, uploads to language registries, or creates/moves tags.
Creating the public Git tag makes the Go module available independently of the
GitHub Release's draft status. Review the prepare-only workflow's assets first,
then create the tag only when Go publication is authorized.
The Go proxy ZIP is a download/test convenience; normal `go get` users obtain the
module through the matching repository tag. Installation from GitHub Release
assets is different from publishing to PyPI, Maven Central or NuGet.org.
