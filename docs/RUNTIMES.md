# Runtime integration

All adapters use the ASCII gRPC metadata key `x-proto-contract` with value `<api>@<semver>`.

## Generate client contracts

The client build's tracked lock is the source of its API identity and version. Run `check` against the proto and lock, then `generate` alongside protobuf binding generation. Regenerate after updating the client lock. The output embeds that version at build time; it does not query a server or negotiate a different version.

```sh
proto-contract generate --lock contracts/orders.json --lang python --out orders_contract.py
proto-contract generate --lock contracts/orders.json --lang go --package orderscontract --out gen/orderscontract/contract.go
proto-contract generate --lock contracts/orders.json --lang java --package contracts.orders --out src/main/java/contracts/orders/Contract.java
proto-contract generate --lock contracts/orders.json --lang dotnet --package Contracts.Orders --out Generated/Orders/Contract.cs
```

| Language | Generated client entry point | Runtime dependency |
| --- | --- | --- |
| Python | `orders_contract.ClientInterceptor()` | `runtimes/python/proto_contract.py` on the import path |
| Go | `orderscontract.ClientInterceptor()` | `github.com/dunkymole/proto-contract/runtimes/go/protocontract` |
| Java | `contracts.orders.Contract.clientInterceptor()` | Compile `runtimes/java/src/main/java` with the generated source |
| .NET | `Contracts.Orders.Contract.ClientInterceptor()` | Include `runtimes/dotnet/ContractClientInterceptor.cs` |
| TypeScript | `contractInterceptor` from its generated module | Connect 2.x; see below |

`--package` sets the Go/Java package or C# namespace. Defaults are `contractgen`, `protocontract.generated`, and `ProtoContract.Generated`, respectively. Java emits a public class named `Contract`, so use `Contract.java`. Generate a distinct module/package/namespace per service. Generated metadata constants are available for inspection, but client setup uses the no-argument entry point. The outputs retain each runtime adapter's existing RPC support; generation does not add streaming support to unary-only runtimes.

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

The adapter sets one contract value, replacing any existing value for that key while preserving other headers. It applies to unary and streaming requests without consuming or wrapping message streams. grpc-bridge preserves this request metadata through retries and connection replacement. Contract enforcement remains on the native gRPC server; the bridge forwards bytes without interpreting the contract. The current demo and server adapters validate unary RPCs only.

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
server = grpc.server(
    executor,
    interceptors=(ContractServerInterceptor("example.orders", "2.5.0"),),
)
```

## Go server

```go
server := grpc.NewServer(
    grpc.UnaryInterceptor(protocontract.UnaryServer("example.orders", "2.5.0")),
)
```

## Go client

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
    new ContractServerInterceptor("example.orders", "2.5.0")));
```

## Java client

```java
OrdersGrpc.OrdersBlockingStub stub = OrdersGrpc.newBlockingStub(channel)
    .withInterceptors(contracts.orders.Contract.clientInterceptor());
```

## .NET server

```csharp
services.AddGrpc(options => options.Interceptors.Add<ContractServerInterceptor>());
services.AddSingleton(new ContractServerInterceptor("example.orders", "2.5.0"));
```

## .NET client

```csharp
var invoker = channel.Intercept(Contracts.Orders.Contract.ClientInterceptor());
var client = new Orders.OrdersClient(invoker);
```
