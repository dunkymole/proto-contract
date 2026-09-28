using Grpc.Core;
using Grpc.Core.Interceptors;
using Grpc.Net.Client;
using ProtoContract;
using ProtoContract.Demo.V1;

var servers = new Dictionary<string,string>{{"Go","go-server:50051"},{"Java","java-server:50053"},{".NET","dotnet-server:50054"},{"Python","python-server:50055"}};

async Task<EchoResponse> Call(string target, string version)
{
    using var channel = GrpcChannel.ForAddress("http://" + target);
    var invoker = channel.Intercept(new ContractClientInterceptor("demo.echo", version));
    var client = new EchoService.EchoServiceClient(invoker);
    return await client.EchoAsync(new EchoRequest { Text = "hello", RequestId = "demo" }, deadline: DateTime.UtcNow.AddSeconds(90));
}

foreach (var (expected,target) in servers)
{
    var response = await Call(target,"1.0.0");
    if (response.Text != "hello" || response.ServerLanguage != expected) throw new Exception($"Unexpected response from {expected}");
    foreach (var rejected in new[]{"1.2.0","2.0.0"}) try { await Call(target,rejected); throw new Exception($"{expected} accepted {rejected}"); }
    catch (RpcException error) when (error.StatusCode == StatusCode.FailedPrecondition) { }
    Console.WriteLine($"PASS .NET client -> {expected} server");
}
Console.WriteLine(".NET client matrix passed");
