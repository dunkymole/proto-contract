# Runtime packages (unpublished)

These are proposed package coordinates. This repository builds them locally; no
registry availability is implied. Package version `0.2.1` versions the adapter
implementation, independently of the API version in each generated contract lock.

| Language | Coordinate | Build/test baseline | Dependency policy |
| --- | --- | --- | --- |
| Go | `github.com/dunkymole/proto-contract/runtimes/go/protocontract` | Go 1.24; module requires Go 1.22 | gRPC 1.71.0; normal Go minimal version selection |
| Python | `proto-contract-runtime` (import `proto_contract`) | Python 3.13; requires Python 3.10+ | grpcio >=1.71,<2 |
| Java | `io.github.dunkymole:proto-contract-runtime` | JDK 21 build, Java 17 bytecode | grpc-api 1.71.0; Maven dependency mediation |
| .NET | `ProtoContract.Runtime` (namespace `ProtoContract`) | .NET 9 SDK; targets net8.0 | Grpc.Core.Api >=2.71,<3 |

The minimum language versions are package requirements, not a claim that every
language/dependency combination has been tested. The matrix uses the baseline
versions above. Compiler and adapter artifacts should initially use the same
release version; any future independent releases must retain generated public
API compatibility or document the required compiler upgrade.

## Build and verify without publication

From the repository root, with Docker Desktop (Linux containers) and PowerShell:

```powershell
./scripts/test-packages.ps1
```

This builds a Python wheel and source distribution, Java binary, source and API-documentation JARs and
POM, a NuGet package, and a Go file-proxy module ZIP under `artifacts/packages/`.
`manifest.json` records SHA-256 checksums, the source revision and whether the
source tree had uncommitted changes when built.
It runs the starter in a fresh virtual environment, then the 20-combination
interoperability matrix. Native client/server containers install the packages;
they do not copy adapter sources into the consumer project. Existing matrix
checks cover all four RPC shapes, invalid metadata, service isolation and strict
dispatch. The Python server also runs its generated-server unit tests.

The Go artifact uses the existing root module, preserving generated imports.
It is an unpublished release candidate for `v0.2.1`. The existing `v0.1.0` and
`v0.2.0` releases and tags remain unchanged. The proxy archive contains
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
mvn install:install-file -Dfile=artifacts/packages/java/proto-contract-runtime-0.2.1.jar -DpomFile=artifacts/packages/java/pom.xml
dotnet nuget add source /absolute/path/to/artifacts/packages/dotnet --name proto-contract-local
dotnet add package ProtoContract.Runtime --version 0.2.1
```

Keep NuGet.org enabled for the transitive gRPC dependency. The container examples
use `packaging/NuGet.Config` to map the runtime specifically to the local feed.

For Go, use a fresh `GOMODCACHE`, set `GOPROXY=file:///absolute/path/to/artifacts/packages/go,https://proxy.golang.org`
and `GONOSUMDB=github.com/dunkymole/proto-contract` in an isolated test environment,
then `go get github.com/dunkymole/proto-contract@v0.2.1`. This bypass is only for
the unpublished local artifact; normal published consumption must use checksums.

Generate each service binding from its build's checked lock, then follow
[runtime wiring](RUNTIMES.md). Install strict dispatch globally, cover every
registered service, and explicitly exempt infrastructure services when needed.
The public API is the existing client/server interceptors, strict dispatch and
coverage helpers used by generated modules. Runtime internals are not a separate
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

The prepared package version is **v0.2.1**; no new tag or Release has been created.
For subsequent versions, update the Python/Java/.NET package metadata, Go proxy
builder, consumer dependency versions, build manifest version and package
verification fixtures together. Commit the tested source and create its matching
`v0.2.1` tag only when release work is authorized. Run this workflow on that
exact tag with `create_draft=true`. The draft job alone receives `contents: write`;
it checks the tag points to the workflow commit, verifies clean-source provenance
and downloaded checksums, then creates an **unpublished draft** with the assets.
An existing release causes creation to fail; assets are never overwritten.

Review the draft and its assets before publishing manually in GitHub. No workflow
step publishes the draft, uploads to language registries, or creates/moves tags.
The Go proxy ZIP is a download/test convenience; normal `go get` users obtain the
module through the matching repository tag. Installation from GitHub Release
assets is different from publishing to PyPI, Maven Central or NuGet.org.
