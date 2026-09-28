# Runtime integration

All adapters use the ASCII gRPC metadata key `x-proto-contract` with value `<api>@<semver>`.

## TypeScript / grpc-bridge client

Proto Contract supports [grpc-bridge](https://github.com/dunkymole/grpc-bridge) and its standard Connect transport/interceptor APIs. Generate each client's interceptor from its tracked contract lock. Use Connect 2.x, grpc-bridge with the public API introduced in [PR #12](https://github.com/dunkymole/grpc-bridge/pull/12), Protobuf-ES 2.x generated service descriptors, and a supported browser or Node.js 24+.

```sh
proto-contract generate --lock contracts/orders.json --lang typescript --out src/gen/orders_contract.ts
proto-contract generate --lock contracts/inventory.json --lang typescript --out src/gen/inventory_contract.ts
```

Each output is a self-contained TypeScript module exporting `contract` (API, version, service) and `contractInterceptor`. It imports only Connect; no copied runtime adapter is needed. Generate it alongside protobuf bindings, rerun when the client lock changes, and use `proto-contract check` to verify that the lock matches the proto. `generate` reads the lock without changing or incrementing its version. Generation currently supports TypeScript.

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
from proto_contract import intercepted_channel

channel = intercepted_channel("orders.example:443", "example.orders", "2.3.0")
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
    grpc.WithUnaryInterceptor(protocontract.UnaryClient("example.orders", "2.3.0")),
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
    .withInterceptors(new ContractClientInterceptor("example.orders", "2.3.0"));
```

## .NET server

```csharp
services.AddGrpc(options => options.Interceptors.Add<ContractServerInterceptor>());
services.AddSingleton(new ContractServerInterceptor("example.orders", "2.5.0"));
```

## .NET client

```csharp
var invoker = channel.Intercept(new ContractClientInterceptor("example.orders", "2.3.0"));
var client = new Orders.OrdersClient(invoker);
```
