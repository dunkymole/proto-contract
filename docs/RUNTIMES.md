# Runtime integration

All adapters use the ASCII gRPC metadata key `x-proto-contract` with value `<api>@<semver>`.

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
