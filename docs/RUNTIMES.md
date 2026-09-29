# Runtime integration

All adapters use the ASCII gRPC metadata key `x-proto-contract` with value `<api>@<semver>`.

Use the [compiler setup and build workflow](COMPILER.md) before the commands below. The Orders/Inventory names in this guide are illustrative application services: replace them with your own locks, generated protobuf types, and package paths. The repository's runnable API is `demo.v1.EchoService`, used in the [README](../README.md#runtime-adapters).

## Generate client and server contracts

Each build's tracked lock is the source of its API identity and version, for both clients and servers. Run `check` against the proto and lock, then `generate` alongside protobuf binding generation. Regenerate after updating that build's lock. Outputs embed the version at build time; they do not query the other endpoint or negotiate a different version. Client and server builds may use different lock versions. Both sides use generated interceptors without supplying API/version strings.

```sh
proto-contract generate --lock contracts/orders.json --lang python --out orders_contract.py
proto-contract generate --lock contracts/orders.json --lang go --package orderscontract --out gen/orderscontract/contract.go
proto-contract generate --lock contracts/orders.json --lang java --package contracts.orders --out src/main/java/contracts/orders/Contract.java
proto-contract generate --lock contracts/orders.json --lang dotnet --package Contracts.Orders --out Generated/Orders/Contract.cs
```

| Language | Generated client | Generated server | Runtime dependency |
| --- | --- | --- | --- |
| Python | `orders_contract.ClientInterceptor()` | `orders_contract.ServerInterceptor()` | `runtimes/python/proto_contract.py` on the import path |
| Go | `orderscontract.ClientInterceptor()` | `orderscontract.ServerInterceptor()` | `github.com/dunkymole/proto-contract/runtimes/go/protocontract` |
| Java | `contracts.orders.Contract.clientInterceptor()` | `contracts.orders.Contract.serverInterceptor()` | Compile `runtimes/java/src/main/java` with the generated source |
| .NET | `Contracts.Orders.Contract.ClientInterceptor()` | `Contracts.Orders.Contract.ServerInterceptor` type | Include both interceptor sources in `runtimes/dotnet` |
| TypeScript | `contractInterceptor` from its generated module | Native backend uses one of the server runtimes above | Connect 2.x; see below |

`--package` sets the Go/Java package or C# namespace. Defaults are `contractgen`, `protocontract.generated`, and `ProtoContract.Generated`, respectively. Java emits a public class named `Contract`, so use `Contract.java`. Generate a distinct module/package/namespace per service. Generated metadata constants are available for inspection; application setup uses the no-argument entry points. The outputs retain each runtime adapter's existing RPC support; generation does not add streaming support to unary-only runtimes.

Register every served contract. Python's generated server interceptor and Go's generated unary server interceptor enforce only their generated service name; register one interceptor for each service (use `grpc.ChainUnaryInterceptor` in Go). Java attaches its generated interceptor with `ServerInterceptors.intercept` on the matching service. In .NET, use the generated server interceptor type with `AddServiceOptions<YourService>` so separate services get separate contract configurations. The generated wrappers own no server or channel lifecycle.

An unrelated service is not protected by a Python/Go interceptor generated for another service. Register all of them explicitly. On clients, keep contract configuration scoped to the intended service: Java supports stub-level `withInterceptors`, .NET uses a service-specific call invoker, and grpc-bridge uses a separate transport wrapper. Python's intercepted channel and Go's connection interceptor attach their contract to every intercepted call; do not reuse those for a different service contract.

## Dependencies and generated files

The generated contract module is additional to ordinary protobuf bindings. Keep it in the source tree or generated-source path compiled by your application. Include both native runtime adapter sources when the module references both client and server types, even in a client-only application. TypeScript output is self-contained apart from Connect.

The tested examples use these dependencies; see the linked build files for exact package versions and protobuf generation commands:

| Runtime | Example setup |
| --- | --- |
| Python | Python 3.13, `grpcio` and `grpcio-tools`; [client requirements](../examples/clients/python/requirements.txt), [server build](../examples/servers/python/Dockerfile) |
| Go | Go 1.24, gRPC-Go and Protobuf; [go.mod](../go.mod), [client build](../examples/clients/go/Dockerfile) |
| Java | Java 21, Maven, gRPC-Java, Protobuf; [client POM](../examples/clients/java/pom.xml), [server POM](../examples/servers/java/pom.xml) |
| .NET | .NET 9, `Grpc.Net.Client`/`Grpc.AspNetCore`, `Grpc.Tools`; [client project](../examples/clients/dotnet/ProtoContractClientDemo.csproj), [server project](../examples/servers/dotnet/ProtoContractServerDemo.csproj) |
| TypeScript | Node.js 24 for the executable example, Connect 2.x, Protobuf-ES 2.x, and pinned grpc-bridge source; [package manifest](../examples/clients/grpc-bridge/package.json), [Docker build](../examples/clients/grpc-bridge/Dockerfile) |

These are demonstrated configurations, not a compatibility promise for every language/runtime release. Python/Java/.NET runtime adapters are source files in this repository; do not assume they are published language packages. grpc-bridge's example package is built locally in Docker from its pinned source revision.

## RPC coverage

The 20-combination interoperability matrix tests unary calls only.

| Adapter | Implemented interception |
| --- | --- |
| Python | Unary-unary client and server hooks |
| Go | Unary client and server hooks |
| .NET | Asynchronous unary client calls and unary server handlers; blocking client and streaming hooks are not implemented |
| Java | Generic gRPC call interception; streaming behavior is not covered by this project's matrix |
| TypeScript / grpc-bridge | Outgoing interceptor can run on all four shapes through `interceptTransport`; the matrix exercises unary calls and focused tests check stream handling |

Do not infer streaming enforcement on a backend from TypeScript's transport support. Generation binds the lock values to existing runtime behavior; it does not implement additional RPC hooks.

## Running the examples

Run `./scripts/test-all.ps1` from the repository root in PowerShell for compiler lock checks, builds, all 20 combinations, and cleanup. Docker must run Linux containers. To run the clients manually in any shell:

```sh
docker compose build
docker compose up -d go-server java-server dotnet-server python-server grpc-bridge
docker compose run --rm -T python-client
docker compose run --rm -T go-client
docker compose run --rm -T java-client
docker compose run --rm -T dotnet-client
docker compose run --rm -T grpc-bridge-client
docker compose down --remove-orphans
```

Stop and inspect a failing build/client command; run cleanup even after a failure. For server diagnostics use `docker compose logs go-server java-server dotnet-server python-server grpc-bridge`. Services are reachable through Compose DNS, not published host ports. Each native client checks the generated compatible contract and deliberately incompatible fixtures against all four servers; grpc-bridge adds older-minor, patch, wrong-API, missing-contract, and concurrent-client checks.

## TypeScript / grpc-bridge client

Proto Contract supports [grpc-bridge](https://github.com/dunkymole/grpc-bridge) and its standard Connect transport/interceptor APIs. Generate each client's interceptor from its tracked contract lock. Use Connect 2.x, grpc-bridge with the public API introduced in [PR #12](https://github.com/dunkymole/grpc-bridge/pull/12), Protobuf-ES 2.x generated service descriptors, and a supported browser or Node.js 24+.

```sh
proto-contract generate --lock contracts/orders.json --lang typescript --out src/gen/orders_contract.ts
proto-contract generate --lock contracts/inventory.json --lang typescript --out src/gen/inventory_contract.ts
```

Each output is a self-contained TypeScript module exporting `contract` (API, version, service) and `contractInterceptor`. It imports only Connect; no copied runtime adapter is needed. Generate it alongside protobuf bindings, rerun when the client lock changes, and use `proto-contract check` to verify that the lock matches the proto. `generate` reads the lock without changing or incrementing its version.

```typescript
import { createClient } from "@connectrpc/connect";
import { contractInterceptor as ordersContract } from "./gen/orders_contract.js";
import { contractInterceptor as inventoryContract } from "./gen/inventory_contract.js";
import { createSharedBridgeConnection, interceptTransport } from "@dunkymole/grpc-bridge";
import { OrdersService, InventoryService } from "./gen/services_pb.js";

const shared = createSharedBridgeConnection({
  url: "wss://bridge.example.com/tunnel",
  target: "orders.example:443",
});
const lease = await shared.acquire();
try {
  const orders = createClient(OrdersService,
    interceptTransport(lease.transport, {
      baseUrl: "https://orders.example",
      interceptors: [ordersContract],
    }));
  const inventory = createClient(InventoryService,
    interceptTransport(lease.transport, {
      baseUrl: "https://orders.example",
      interceptors: [inventoryContract],
    }));
  // Use both clients while holding the lease. Their contracts remain independent.
} finally {
  await lease.release();
  await shared.dispose(); // When the owner shuts down.
}
```

Use the same per-client wrapper with `openBridgeConnection` or `createBridgeConnection`. Contract configuration belongs to the generated service client, not a shared channel: create a separate wrapper for each API/version. Wrappers copy call headers and do not mutate the underlying transport or caller-owned headers. Close connections, release shared leases, and dispose shared handles when their owner shuts down.

The adapter sets one contract value, replacing any existing value for that key while preserving other headers. It applies to unary and streaming requests without consuming or wrapping message streams. grpc-bridge preserves this request metadata through retries and connection replacement. Contract enforcement remains on the native gRPC server; the bridge forwards bytes without interpreting the contract. End-to-end validation in this project covers unary RPCs; backend hooks differ as described in [RPC coverage](#rpc-coverage).

The generated `contractInterceptor` is a standard Connect `Interceptor` and rejects use with a different service. Apply it per client using grpc-bridge's public `interceptTransport()`; do not register a fixed contract on the shared connection. No custom transport implementation or private bridge imports are needed. `baseUrl` identifies the logical backend to interceptors and does not change routing or open another connection.

The generic `contractClientInterceptor(api, version)` in `runtimes/typescript` remains a low-level tool for explicit compatibility test fixtures. Normal application code uses the generated module. Its version describes the schema used to build that client, not the deployed server's latest version.

Client interceptors run before connection interceptors, once per logical RPC outside retries; responses unwind in reverse order. Connection-level interceptors remain available for tracing or authentication. The last `header.set()` wins, so a connection interceptor must not overwrite the client's contract. Shared defaults should only set a header when it is absent. Wrappers own no connection or lease; retain the underlying connection or lease for the clients' lifetime.

The [Docker example](../examples/clients/grpc-bridge/README.md) builds the unpublished bridge package from pinned source and tests TypeScript through the bridge against Go, Java, .NET, and Python servers.

## Python client

```python
from orders_contract import ClientInterceptor
import grpc

channel = grpc.intercept_channel(
    grpc.secure_channel("orders.example:443", grpc.ssl_channel_credentials()),
    ClientInterceptor(),
)
stub = OrdersStub(channel)
```

## Python server

```python
from orders_contract import ServerInterceptor

server = grpc.server(
    executor,
    interceptors=(ServerInterceptor(),),
)
```

## Go server

```go
server := grpc.NewServer(
    grpc.UnaryInterceptor(orderscontract.ServerInterceptor()),
)
```

## Go client

Import `orderscontract` from the module path where you generated `gen/orderscontract/contract.go`. Supply your normal transport credentials and close the connection when the client shuts down.

```go
connection, _ := grpc.NewClient(target,
    grpc.WithTransportCredentials(credentials),
    grpc.WithUnaryInterceptor(orderscontract.ClientInterceptor()),
)
```

## Java server

```java
builder.addService(ServerInterceptors.intercept(
    new OrdersService(),
    contracts.orders.Contract.serverInterceptor()));
```

## Java client

```java
OrdersGrpc.OrdersBlockingStub stub = OrdersGrpc.newBlockingStub(channel)
    .withInterceptors(contracts.orders.Contract.clientInterceptor());
```

## .NET server

```csharp
services.AddGrpc().AddServiceOptions<OrdersService>(options =>
    options.Interceptors.Add<Contracts.Orders.Contract.ServerInterceptor>());
```

## .NET client

```csharp
var invoker = channel.Intercept(Contracts.Orders.Contract.ClientInterceptor());
var client = new Orders.OrdersClient(invoker);
```
