# Runtime integration

All adapters use the ASCII gRPC metadata key `x-proto-contract` with value `<api>@<semver>`.

## Python client

```python
from proto_contract import intercepted_channel

channel = intercepted_channel("orders.example:443", "example.orders", "2.3.0")
stub = OrdersStub(channel)
```

## Go server

```go
server := grpc.NewServer(
    grpc.UnaryInterceptor(protocontract.UnaryServer("example.orders", "2.5.0")),
)
```

## Java server

```java
builder.addService(ServerInterceptors.intercept(
    new OrdersService(),
    new ContractServerInterceptor("example.orders", "2.5.0")));
```

## .NET server

```csharp
services.AddGrpc(options => options.Interceptors.Add<ContractServerInterceptor>());
services.AddSingleton(new ContractServerInterceptor("example.orders", "2.5.0"));
```

## C++ server

The public C++ interceptor API can observe metadata but cannot reliably end the call before the handler. Call the verifier as the handler's first operation:

```cpp
grpc::Status Echo(grpc::ServerContext* context, const Request* request, Response* response) override {
  auto result = proto_contract::Verify(*context, "example.orders", "2.5.0");
  if (!result.ok()) return result;
  // application logic
}
```

For a TLS server, an `AuthMetadataProcessor` can enforce the same check before dispatch; it is tied to server credentials and is therefore less composable for this library's general adapter.
