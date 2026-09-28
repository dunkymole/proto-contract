# TypeScript client through grpc-bridge

This example uses grpc-bridge's public `interceptTransport()` with a generated `contractInterceptor` to bind Proto Contract metadata to each generated Connect service client over a shared grpc-bridge connection. Each RPC travels from Node.js 24 over a WebSocket tunnel, through the Go bridge, to a native gRPC server.

The Docker build checks the proto against `contracts/demo.echo.json` and runs `proto-contract generate --lock /contracts/demo.echo.json --lang typescript --out gen/echo_contract.ts`. The normal client imports this generated interceptor without specifying an API name or version. Remaining explicit versions are deliberate matrix test fixtures for older, newer, and incompatible clients.

From the repository root, run the complete 20-combination matrix:

```powershell
./scripts/test-all.ps1
```

Or run only the four grpc-bridge combinations:

```sh
docker compose build grpc-bridge grpc-bridge-client go-server java-server dotnet-server python-server
docker compose up -d grpc-bridge go-server java-server dotnet-server python-server
docker compose run --rm -T grpc-bridge-client
docker compose down
```

The client verifies the response and server identity for older-minor, equal, and different-patch compatible versions. It requires `FAILED_PRECONDITION` for newer-minor, different-major, wrong-API, and missing contracts. Concurrent clients with independent API/version settings share one connection per backend, and a compatible client still succeeds after rejected calls. Calls use a 90-second deadline and wait for servers to become ready; each connection is closed after its backend checks finish.

`Dockerfile` builds both the bridge executable and its unpublished TypeScript package from commit `9872bb4b46abb510f1a1fa3416275148a533357b` (the public per-client interceptor API in grpc-bridge PR #12). The package lock pins the resulting tarball and npm dependencies; updating `GRPC_BRIDGE_REF` also requires regenerating the lock against the new tarball. Protobuf-ES bindings are generated from this repository's demo proto at build time. Type-checking and focused interceptor tests run before the image is produced.

`targets.json` explicitly allows the four demo backends. The bridge is available only on the Compose network. In an application, use your own bridge URL, target allowlist, TLS and authentication settings; the reusable adapter is `runtimes/typescript/proto-contract.ts`.
