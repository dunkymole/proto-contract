# Contributing

Thank you for helping improve Proto Contract.

1. Open an issue for substantial behavior or policy changes so the compatibility implications can be agreed first.
2. Keep the lock format deterministic and backward-readable.
3. Add focused tests for classifier or generator changes and extend the cross-language demo for runtime changes.
4. Run `go test ./...` and `./scripts/test-all.ps1` before submitting code changes. For documentation-only changes, verify the affected commands, examples, and links; report which checks you ran.

## Development setup

The compiler uses Go (the repository selects a Go 1.24 toolchain). The complete demo needs Docker Compose v2 with Linux containers and PowerShell; its language toolchains run inside the containers. See [compiler setup](docs/COMPILER.md) for local `protoc` requirements and [runtime integration](docs/RUNTIMES.md) for each adapter's dependencies.

Run checks from the repository root. A Docker-only Go test command is:

```powershell
docker run --rm -v "${PWD}:/src" -w /src golang:1.24-bookworm go test ./...
```

For a schema change, run `check`, review the calculated bump, and deliberately `update` the tracked lock. Generated contracts must derive from that lock on both clients and servers. Do not introduce handwritten API/version arguments into application examples; explicit mismatches belong only in compatibility tests.

After changing the generator or demo lock, regenerate the checked-in Go contract before running Go tests:

```sh
go run ./cmd/proto-contract generate --lock contracts/demo.echo.json --lang go --package echocontract --out gen/go/contracts/echo/contract.go
```

The other demo contract modules are generated during Docker builds. The matrix script also runs the compiler lock check, language builds, Python generated-server tests, TypeScript type-checking/tests, and all 20 client/server combinations. It cleans up its Compose containers and network afterward.

Use clear commits and explain the user-visible behavior in the pull request. By contributing, you agree that your contribution is licensed under the MIT License.
