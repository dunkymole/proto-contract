# Runtime integration

All native adapters use the ASCII gRPC metadata key `x-proto-contract` with exactly one value, `<api>@<MAJOR.MINOR.PATCH>`. The API is 1–64 ASCII characters from `[A-Za-z0-9._:/-]`; this preserves identifiers such as `example/orders:v1`. The complete metadata value is at most 128 ASCII bytes. Version components are canonical nonnegative decimal integers no larger than `2147483647` (`0` or a nonzero digit followed by digits); signs, leading zeroes, whitespace, Unicode lookalikes, extra separators, and overflow are rejected. Every adapter applies the same rules at configuration construction and call validation.

Use the [compiler setup and build workflow](COMPILER.md) before the commands below. The Orders/Inventory names in this guide are illustrative application services: replace them with your own locks, generated protobuf types, and package paths. The repository's runnable API is `demo.v1.EchoService`, used in the [README](../README.md#runtime-adapters).

## Generate client and server contracts

Each build's tracked lock is the source of its API identity and version, for both clients and servers. Run `check` against the proto and lock, then generate native adapters from the lock. The TypeScript grpc-bridge client uses the protoc plugin so it validates that lock against the same descriptors used for the generated service binding. Regenerate after updating that build's lock. Outputs embed the version at build time; they do not query the other endpoint or negotiate a different version. Client and server builds may use different lock versions.

```sh
proto-contract generate --lock contracts/orders.json --lang python --proto proto/orders/v1/orders.proto --proto-path proto --out orders_contract.py
proto-contract generate --lock contracts/orders.json --lang go --package orderscontract --proto proto/orders/v1/orders.proto --proto-path proto --out gen/orderscontract/contract.go
proto-contract generate --lock contracts/orders.json --lang java --package contracts.orders --proto proto/orders/v1/orders.proto --proto-path proto --out src/main/java/contracts/orders/Contract.java
proto-contract generate --lock contracts/orders.json --lang dotnet --package Contracts.Orders --proto proto/orders/v1/orders.proto --proto-path proto --target client --out Generated/Orders/Contract.cs
```

| Language | Generated client | Generated server | Runtime dependency |
| --- | --- | --- | --- |
| Python | `orders_contract.client(channel)` | `orders_contract.add_to_server(builder, impl)` | Installed `proto-contract-runtime` wheel |
| Go | `orderscontract.NewClient(conn)` | `orderscontract.RegisterServer(builder, impl)` | `github.com/dunkymole/proto-contract/runtimes/go/protocontract` |
| Java | `Contract.blockingClient(channel)` | `Contract.addService(builder, impl)` | `io.github.dunkymole:proto-contract-runtime` dependency |
| .NET | `Contract.CreateClient(channel)` | `Contract.MapService<Impl>(app)` | `ProtoContract.Runtime`; servers also use `ProtoContract.AspNetCore` |
| TypeScript | `connection.client(contract)` from the generated strict artifact | Native backend uses one of the server runtimes above | Connect 2.x, Protobuf-ES 2.15.0, strict grpc-bridge package; see below |

`--package` sets the Go/Java package or C# namespace. Defaults are `contractgen`, `protocontract.generated`, and `ProtoContract.Generated`, respectively. Java emits a public class named `Contract`, so use `Contract.java`. Generate a distinct module/package/namespace per service. Generated metadata constants are available for inspection; combined helpers need no API or version arguments. Native Go, Python, Java, and .NET adapters enforce unary-unary, client-streaming, server-streaming, and bidirectional RPCs.

Use the combined registration helpers below. Each server builder collects service/contract pairs and installs one global strict dispatcher when built. .NET installs it with `AddContractGrpc()` and collects pairs during endpoint mapping, freezing contracts when the host starts. A registered service without a contract is denied before application handlers run, including services registered through ordinary gRPC APIs. Registration helpers reject mismatched service names and duplicate contracts. Configure builders before building; they are single-use and not intended for concurrent mutation. .NET endpoint conventions validate the actual protobuf service name during endpoint construction.

Typed helpers bind protection to each client, including all streaming shapes, without changing the shared connection. Protobuf symbols are inferred during descriptor-based generation; application code passes only a connection or implementation. Register multiple generated contracts on the same server builder; no manual dispatch map or unary/stream wiring is needed. Python/Go server builders and Java's `ContractServerBuilder` accept explicit fully-qualified exemptions at construction; .NET accepts them in `AddContractGrpc(...)`. Register exempt health/reflection services through ordinary gRPC APIs on the resulting server (or Java's underlying builder). The owner still closes channels, starts/stops servers, and configures transport security. Low-level interceptors remain available for advanced integration and negative tests.

### Bind protobuf types at generation time

The native protoc plugin infers protobuf imports and service types by default.
Standalone generation does the same when given `--proto` and `--proto-path`; the
service is taken from the lock. Neither path needs `service_import` or
`service_export` for a conventional build.

| Language | Import/namespace inference | Service type inference |
| --- | --- | --- |
| Go | `go_package`, excluding its optional `;alias` suffix | Public Go protobuf generator naming API |
| Python | Descriptor file path with hyphens normalized and `_pb2_grpc` suffix; the proto package does not determine imports | Original service name |
| Java | `java_package`, otherwise the proto package; independent of `java_multiple_files` | Original service name plus `Grpc` |
| .NET | `csharp_namespace` when explicitly set, otherwise the standard namespace conversion of the proto package; empty/global namespaces are supported | Original service name |

```json
{
  "service": "demo.v1.EchoService",
  "lock": "demo.echo.json",
  "output": "generated/python/echo_contract.py"
}
```

`service_import` and `service_export` remain independent optional overrides (or
`--service-import` and `--service-export` on the CLI). Override only the part that
differs from the descriptors. For example, the Go consumer examples use
`"service_import": "starter.local/demo/gen/go/demo/v1"` because their generated
files live in a different module from the proto's `go_package`. External Go `M`
mappings, Python prefix stripping, and relocated/custom bindings must be supplied
explicitly: the plugin cannot see another generator's command-line options.

Descriptor-based generation validates the exact lock, then fails with an actionable
error if a binding cannot be inferred or represented. It never silently emits
generic helpers. Missing Go `go_package` requires an import override. Java's unnamed
package cannot be referenced from a named contract package. Python filenames with
dots inside path components need an explicit import and matching relocation;
keyword service names emitted as invalid source by Python/C# gRPC are rejected.
Naming conformance tests compile/import actual protobuf outputs, including
underscores, leading underscores, acronym/digit combinations, namespace defaults,
explicit namespaces and Python hyphenated paths: run `./scripts/test-naming.ps1`.

Legacy standalone generation without `--proto` remains available: pass both
binding flags for typed helpers, or neither for the low-level generic API. For a
global C# namespace, provide the service export and omit the import (or pass
`--service-import ""`).
Partial overrides otherwise need descriptors. TypeScript still requires its explicit module/export
bindings, since its generated module layout is configured by Protobuf-ES.

For .NET, set `target` (`--target`) to `client` or `server` to match `GrpcServices`
in the project. Client output references only the generated client and core runtime;
server output adds a typed `MapService<TService>(app)` constrained to that protobuf
service's base class and requires `ProtoContract.AspNetCore`. The default `both`
requires both protobuf bindings and the server integration package. Java provides
`client(channel)`, `blockingClient(channel)`, and `futureClient(channel)`.

## Dependencies and generated files

The generated contract module is additional to ordinary protobuf bindings. Keep it in the source tree or generated-source path compiled by your application. Install the native runtime package, which includes client and server types, even in a client-only application. TypeScript contract artifacts import `defineContract` and `RuntimeGraph` from grpc-bridge's `/codegen` entry and the Protobuf-ES descriptor emitted alongside them.

The tested examples use these dependencies; see the linked build files for exact package versions and protobuf generation commands:

| Runtime | Example setup |
| --- | --- |
| Python | Python 3.13, `grpcio` and `grpcio-tools`; [client requirements](../examples/clients/python/requirements.txt), [server build](../examples/servers/python/Dockerfile) |
| Go | Go 1.27.2, gRPC-Go and Protobuf; [go.mod](../go.mod), [client build](../examples/clients/go/Dockerfile) |
| Java | Java 21, Maven, gRPC-Java, Protobuf; [client POM](../examples/clients/java/pom.xml), [server POM](../examples/servers/java/pom.xml) |
| .NET | .NET 9, `Grpc.Net.Client`/`Grpc.AspNetCore`, `Grpc.Tools`; [client project](../examples/clients/dotnet/ProtoContractClientDemo.csproj), [server project](../examples/servers/dotnet/ProtoContractServerDemo.csproj) |
| TypeScript | Node.js 24 for the executable example, Connect 2.x, Protobuf-ES 2.15.0, and pinned grpc-bridge source; [package manifest](../examples/clients/grpc-bridge/package.json), [Docker build](../examples/clients/grpc-bridge/Dockerfile) |

These are demonstrated configurations, not a compatibility promise for every language/runtime release. Native runtime packages are built locally and installed from artifacts by the examples. v0.6.0 packages are available from [GitHub Releases](https://github.com/dunkymole/proto-contract/releases/tag/v0.6.0); see [installation and package preparation](PACKAGES.md). grpc-bridge's example package is built locally in Docker from its pinned source revision.

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
pwsh ./scripts/build-packages.ps1
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

Install the compiler and plugin with `npm install --save-dev @dunkymole/proto-contract@0.6.0`. Within an npm script, the commands are on `PATH`; let protoc discover `protoc-gen-proto-contract` there. Outside npm scripts, use `npx proto-contract` for compiler commands. `protoc` and the Protobuf-ES generator are separate tools. The npm package supplies build tooling only; the runtime still comes from grpc-bridge. Use the reviewed source package described below until that separate runtime is published to npm.

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
import orders_contract
import grpc

channel = grpc.secure_channel("orders.example:443", grpc.ssl_channel_credentials())
stub = orders_contract.client(channel)
```

## Python server

```python
from proto_contract import ServerBuilder
import orders_contract

builder = ServerBuilder()
orders_contract.add_to_server(builder, implementation)
server = builder.build(executor)
```

## Go server

```go
builder := protocontract.NewServerBuilder()
if err := orderscontract.RegisterServer(builder, implementation); err != nil { log.Fatal(err) }
server, err := builder.Build()
if err != nil { log.Fatal(err) }
```

`Build` reserves gRPC's single unary and stream interceptor slots for the strict
guard. Add application interceptors with `grpc.ChainUnaryInterceptor(...)` and
`grpc.ChainStreamInterceptor(...)` options to `Build`; they run after contract
validation, even if they return cached responses without invoking a handler.
Passing `grpc.UnaryInterceptor(...)` or `grpc.StreamInterceptor(...)` instead is
rejected by gRPC as duplicate interceptor configuration.

## Go client

Import `orderscontract` from the module path where you generated `gen/orderscontract/contract.go`. Supply your normal transport credentials and close the connection when the client shuts down.

```go
connection, err := grpc.NewClient(target, grpc.WithTransportCredentials(credentials))
if err != nil { log.Fatal(err) }
client := orderscontract.NewClient(connection)
```

## Java server

```java
var builder = new ContractServerBuilder(ServerBuilder.forPort(50051));
contracts.orders.Contract.addService(builder, new OrdersService());
var server = builder.build();
```

## Java client

```java
var stub = contracts.orders.Contract.blockingClient(channel);
var asyncStub = contracts.orders.Contract.client(channel);
```

## .NET server

```csharp
// Install ProtoContract.AspNetCore and import the ProtoContract namespace.
builder.Services.AddContractGrpc();
var app = builder.Build();
Contracts.Orders.Contract.MapService<OrdersService>(app);
```

## .NET client

```csharp
var client = Contracts.Orders.Contract.CreateClient(channel);
```

## Edition 2023 descriptors

The compiler supports the [core Edition 2023 subset](COMPATIBILITY.md#protobuf-edition-2023). TypeScript runtime graph checks include resolved presence, packing, UTF-8 validation, enum openness, and message encoding; JSON policy and source annotations are checked against descriptors during generation. Native contract generation supports edition descriptors, but compiling the protobuf bindings requires edition-capable native generators and runtimes. Existing cross-language interoperability examples continue to use proto3.
