# Runtime integration

All native adapters use the ASCII gRPC metadata key `x-proto-contract` with exactly one value, `<api>@<MAJOR.MINOR.PATCH>`. The API is 1–64 ASCII characters from `[A-Za-z0-9._:/-]`; this preserves identifiers such as `example/orders:v1`. The complete metadata value is at most 128 ASCII bytes. Version components are canonical nonnegative decimal integers no larger than `2147483647` (`0` or a nonzero digit followed by digits); signs, leading zeroes, whitespace, Unicode lookalikes, extra separators, and overflow are rejected. Every adapter applies the same rules at configuration construction and call validation.

Use the [compiler setup and build workflow](COMPILER.md) before the commands below. The Orders/Inventory names in this guide are illustrative application services: replace them with your own locks, generated protobuf types, and package paths. The repository's runnable API is `demo.v1.EchoService`, used in the [README](../README.md#runtime-adapters).

## Generate client and server contracts

Each build's tracked lock is the source of its API identity and version, for both clients and servers. Run `check` against the proto and lock, then generate native adapters from the lock. The TypeScript grpc-bridge client uses the protoc plugin so it validates that lock against the same descriptors used for the generated service binding. Regenerate after updating that build's lock. Outputs embed the version at build time; they do not query the other endpoint or negotiate a different version. Client and server builds may use different lock versions.

```sh
proto-contract generate --lock contracts/orders.json --lang python --out orders_contract.py
proto-contract generate --lock contracts/orders.json --lang go --package orderscontract --out gen/orderscontract/contract.go
proto-contract generate --lock contracts/orders.json --lang java --package contracts.orders --out src/main/java/contracts/orders/Contract.java
proto-contract generate --lock contracts/orders.json --lang dotnet --package Contracts.Orders --out Generated/Orders/Contract.cs
```

| Language | Generated client | Generated server | Runtime dependency |
| --- | --- | --- | --- |
| Python | `orders_contract.ClientInterceptor()` | `orders_contract.StrictServerInterceptor()` | `runtimes/python/proto_contract.py` on the import path |
| Go | `orderscontract.ClientInterceptor()` | `orderscontract.StrictServer()` | `github.com/dunkymole/proto-contract/runtimes/go/protocontract` |
| Java | `contracts.orders.Contract.clientInterceptor()` | `contracts.orders.Contract.strictServerInterceptor()` | Compile `runtimes/java/src/main/java` with the generated source |
| .NET | `Contracts.Orders.Contract.ClientInterceptor()` | `Contracts.Orders.Contract.StrictServerInterceptor()` | Include runtime interceptor sources in `runtimes/dotnet` |
| TypeScript | `connection.client(contract)` from the generated strict artifact | Native backend uses one of the server runtimes above | Connect 2.x, Protobuf-ES 2.15.0, strict grpc-bridge package; see below |

`--package` sets the Go/Java package or C# namespace. Defaults are `contractgen`, `protocontract.generated`, and `ProtoContract.Generated`, respectively. Java emits a public class named `Contract`, so use `Contract.java`. Generate a distinct module/package/namespace per service. Generated metadata constants are available for inspection; application setup uses the no-argument entry points. Native Go, Python, Java, and .NET adapters enforce unary-unary, client-streaming, server-streaming, and bidirectional RPCs.

Use the generated strict dispatcher for servers that must fail closed. It owns a map from fully-qualified protobuf service names to each service's contract interceptor and rejects calls to any registered service missing from that map before reaching application handlers. Add intentional exemptions by fully-qualified name only. Install it globally: Python's `StrictServerInterceptor` in `grpc.server(..., interceptors=...)`, Go's generated `StrictServer` with both unary and stream chains, Java's `ServerBuilder.intercept(...)`, or .NET's global `AddGrpc(options => options.Interceptors.Add<StrictServerInterceptor>())`. Go can additionally call `ValidateRegisteredServices(server)` after registering services to detect a missing entry at startup; other adapters enforce the same rule at dispatch. Do not attach strict enforcement only to selected services, since a later service could otherwise bypass it. The generated wrappers own no server or channel lifecycle.

On clients, generated interceptors are service-scoped and leave calls to other services unchanged, which lets independently generated clients share a channel. For multiple server contracts, combine each generated service interceptor in the strict dispatcher's contract map. Java and .NET examples use global strict dispatch so adding an unlisted service is denied. The dispatcher lets explicitly exempt services proceed without contract metadata; exemptions should be deliberate and kept narrow.

## Dependencies and generated files

The generated contract module is additional to ordinary protobuf bindings. Keep it in the source tree or generated-source path compiled by your application. Include both native runtime adapter sources when the module references both client and server types, even in a client-only application. TypeScript contract artifacts import `defineContract` and `RuntimeGraph` from grpc-bridge's `/codegen` entry and the Protobuf-ES descriptor emitted alongside them.

The tested examples use these dependencies; see the linked build files for exact package versions and protobuf generation commands:

| Runtime | Example setup |
| --- | --- |
| Python | Python 3.13, `grpcio` and `grpcio-tools`; [client requirements](../examples/clients/python/requirements.txt), [server build](../examples/servers/python/Dockerfile) |
| Go | Go 1.24, gRPC-Go and Protobuf; [go.mod](../go.mod), [client build](../examples/clients/go/Dockerfile) |
| Java | Java 21, Maven, gRPC-Java, Protobuf; [client POM](../examples/clients/java/pom.xml), [server POM](../examples/servers/java/pom.xml) |
| .NET | .NET 9, `Grpc.Net.Client`/`Grpc.AspNetCore`, `Grpc.Tools`; [client project](../examples/clients/dotnet/ProtoContractClientDemo.csproj), [server project](../examples/servers/dotnet/ProtoContractServerDemo.csproj) |
| TypeScript | Node.js 24 for the executable example, Connect 2.x, Protobuf-ES 2.15.0, and pinned grpc-bridge source; [package manifest](../examples/clients/grpc-bridge/package.json), [Docker build](../examples/clients/grpc-bridge/Dockerfile) |

These are demonstrated configurations, not a compatibility promise for every language/runtime release. Python/Java/.NET runtime adapters are source files in this repository; do not assume they are published language packages. grpc-bridge's example package is built locally in Docker from its pinned source revision.

## RPC coverage

The interoperability matrix exercises all four RPC shapes across Python, Go, Java, and .NET backends, plus the TypeScript grpc-bridge client. Focused tests use a shared malformed and boundary-case metadata vector corpus and make real streaming calls with accepted, missing, and incompatible contracts, asserting rejected calls never reach application handlers.

| Adapter | Implemented interception |
| --- | --- |
| Python | Unary-unary, client-streaming, server-streaming, and bidirectional client and server hooks |
| Go | Unary, client-streaming, server-streaming, and bidirectional client and server hooks |
| .NET | Blocking and asynchronous unary plus all streaming client and server hooks |
| Java | Unary and all streaming client/server calls through gRPC call interception |
| TypeScript / grpc-bridge | Generated strict clients bind the service descriptor and stamp contract metadata for all four shapes; backend enforcement is performed by the native server runtime |

The metadata parser is deliberately bounded and portable; malformed or duplicated values fail closed instead of being normalized.

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

Proto Contract generates contract-bound clients for [grpc-bridge](https://github.com/dunkymole/grpc-bridge). The protoc plugin checks each lock against the exact descriptor request used to generate Protobuf-ES bindings, then imports and binds that service descriptor in the generated artifact. The runtime graph projection is tested against Protobuf-ES 2.15.0; use Connect 2.x, the strict grpc-bridge package entry, that Protobuf-ES version, and a supported browser or Node.js 24+.

```sh
protoc -I proto \
  --plugin=protoc-gen-es=node_modules/.bin/protoc-gen-es \
  --es_out=gen --es_opt=target=ts,import_extension=js \
  --plugin=protoc-gen-proto-contract=protoc-gen-proto-contract \
  --proto-contract_out=. \
  --proto-contract_opt=lang=typescript,bindings=contracts/typescript-bindings.json \
  orders/v1/orders.proto inventory/v1/inventory.proto
```

The bindings JSON has one entry per service with explicit `service`, `lock`, `output`, `service_import`, and `service_export` fields. Lock paths resolve relative to the JSON file; outputs are protoc-relative. The generated module exports a `contract` and the runtime graph used for deliberate test fixtures. TypeScript generation from a lock alone is rejected because it cannot verify which protobuf descriptor the client actually uses.

```typescript
import { createSharedBridgeConnection } from "@dunkymole/grpc-bridge";
import { contract as ordersContract } from "./gen/orders_contract.js";
import { contract as inventoryContract } from "./gen/inventory_contract.js";

const shared = createSharedBridgeConnection({
  url: "wss://bridge.example.com/tunnel",
  target: "orders.example:443",
});
const lease = await shared.acquire();
try {
  const orders = lease.client(ordersContract);
  const inventory = lease.client(inventoryContract);
  // Use both clients while holding the lease. Their contracts remain independent.
} finally {
  await lease.release();
  await shared.dispose(); // When the owner shuts down.
}
```

Use the same `connection.client(contract)` API with `openBridgeConnection` or `createBridgeConnection`. Contract configuration belongs to the generated client, not a shared connection. The main package entry does not expose a raw transport; generic transports remain under the explicit `/raw` escape hatch. Close connections, release shared leases, and dispose shared handles when their owner shuts down.

The generated artifact checks its imported Protobuf-ES descriptor graph at module initialization and the client guard validates each actual method before stamping metadata. It applies to unary and streaming calls, keeps separate service contracts isolated on a shared lease, and preserves metadata through retry handling. Native strict server dispatch enforces the corresponding server lock; the bridge only forwards bytes.

The generic `contractClientInterceptor(api, version)` and `/raw` package entry are reserved for explicit negative/compatibility tests and migrations, not normal application clients.

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
from orders_contract import StrictServerInterceptor

server = grpc.server(
    executor,
    interceptors=(StrictServerInterceptor(),),
)
```

## Go server

```go
strict, err := orderscontract.StrictServer() // accepts explicit service exemptions if needed
if err != nil { log.Fatal(err) }
server := grpc.NewServer(grpc.ChainUnaryInterceptor(strict.Unary()), grpc.ChainStreamInterceptor(strict.Stream()))
orderspb.RegisterOrdersServer(server, implementation)
if err := strict.ValidateRegisteredServices(server); err != nil { log.Fatal(err) }
```

## Go client

Import `orderscontract` from the module path where you generated `gen/orderscontract/contract.go`. Supply your normal transport credentials and close the connection when the client shuts down.

```go
connection, _ := grpc.NewClient(target,
    grpc.WithTransportCredentials(credentials),
    grpc.WithUnaryInterceptor(orderscontract.ClientInterceptor()),
    grpc.WithStreamInterceptor(orderscontract.ClientStreamInterceptor()),
)
```

## Java server

```java
builder.addService(new OrdersService());
builder.intercept(contracts.orders.Contract.strictServerInterceptor());
```

## Java client

```java
OrdersGrpc.OrdersBlockingStub stub = OrdersGrpc.newBlockingStub(channel)
    .withInterceptors(contracts.orders.Contract.clientInterceptor());
```

## .NET server

```csharp
services.AddSingleton(Contracts.Orders.Contract.StrictServerInterceptor());
services.AddGrpc(options => options.Interceptors.Add<ProtoContract.StrictServerInterceptor>());
```

## .NET client

```csharp
var invoker = channel.Intercept(Contracts.Orders.Contract.ClientInterceptor());
var client = new Orders.OrdersClient(invoker);
```
