# TypeScript client through grpc-bridge

This example uses grpc-bridge's strict generated-contract API. The application creates the Echo client with `connection.client(contract)`; each RPC travels from Node.js 24 over a WebSocket tunnel, through the Go bridge, to a native gRPC server.

The Docker build invokes protoc once for Protobuf-ES bindings and the contract plugin, so the generated artifact imports the same service descriptor it was validated against. It reads `contracts/typescript-bindings.json`, checks the full lock digest against the request descriptors, and emits the runtime graph and lock identity. Ordinary client calls use only the generated contract. Explicit API/version values are limited to deliberate compatibility and rejection fixtures; the missing-contract case uses the explicit `/raw` escape hatch.

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

`Dockerfile` builds both the bridge executable and its unpublished TypeScript package from the exact strict API commit in `GRPC_BRIDGE_REF` (default `80ecc2143dd335c6847e24a32ad510d491f6ea24`). `GRPC_BRIDGE_REPOSITORY` can select a reviewed fork or local Git server; changing the source revision requires regenerating the lock against the resulting package tarball. Protobuf-ES and Proto Contract bindings are generated together from this repository's demo proto at build time. Type-checking and focused tests run before the image is produced.

`targets.json` explicitly allows the four demo backends. The bridge is available only on the Compose network. In an application, use your own bridge URL, target allowlist, TLS and authentication settings, then pass the generated contract artifact to `connection.client()`. `runtimes/typescript/proto-contract.ts` is used only for deliberate raw negative tests in this example.

Only unary end-to-end enforcement is demonstrated here. See [runtime coverage](../../../docs/RUNTIMES.md#rpc-coverage) and the [compiler workflow](../../../docs/COMPILER.md) for the complete build sequence. Run the script in PowerShell; the manual Docker commands above also work in other shells. Always run `docker compose down` after a manual run, including after failures.
