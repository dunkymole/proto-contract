using Grpc.Core;
using Grpc.Core.Interceptors;
using Grpc.Net.Client;
using ProtoContract;
using EchoContract = ProtoContract.Generated.Echo.Contract;
using ProtoContract.Demo.V1;
using Google.Protobuf;
using System.Text.Json;

var servers = new Dictionary<string,string>{{"Go","go-server:50051"},{"Java","java-server:50053"},{".NET","dotnet-server:50054"},{"Python","python-server:50055"}};

void VerifyContractVectors()
{
    using var document = JsonDocument.Parse(File.ReadAllText("/contract-vectors.json"));
    var records = document.RootElement.EnumerateArray().ToArray();
    foreach (var vector in records)
    {
        var api = vector.GetProperty("api").GetString()!;
        var version = vector.GetProperty("server").GetString()!;
        var values = vector.GetProperty("values").EnumerateArray().Select(item => item.GetString()!).ToArray();
        var expected = vector.GetProperty("compatible").GetBoolean();
        bool compatible;
        try { compatible = ContractServerInterceptor.CompatibilityError(api, version, values) is null; }
        catch (ArgumentException) { compatible = false; }
        if (compatible != expected) throw new Exception("contract vector failed: " + vector.GetProperty("name").GetString());
    }
    if (records.Length != 29) throw new Exception($"Expected 29 contract vectors, found {records.Length}");
}
VerifyContractVectors();

void VerifyBlockingUnaryInterceptor()
{
    var method = new Method<EchoRequest, EchoResponse>(MethodType.Unary, EchoContract.Service, "Echo",
        Marshallers.Create<EchoRequest>((request, context) => context.Complete(request.ToByteArray()), context => EchoRequest.Parser.ParseFrom(context.PayloadAsNewBuffer())),
        Marshallers.Create<EchoResponse>((response, context) => context.Complete(response.ToByteArray()), context => EchoResponse.Parser.ParseFrom(context.PayloadAsNewBuffer())));
    var context = new ClientInterceptorContext<EchoRequest, EchoResponse>(method, null, new CallOptions());
    var interceptor = new ContractClientInterceptor(EchoContract.Api, EchoContract.Version, EchoContract.Service);
    _ = interceptor.BlockingUnaryCall(new EchoRequest(), context, (request, stamped) =>
    {
        var value = stamped.Options.Headers?.Get(ContractServerInterceptor.MetadataKey)?.Value;
        if (value != $"{EchoContract.Api}@{EchoContract.Version}") throw new Exception("blocking unary interceptor did not stamp the contract");
        return new EchoResponse();
    });
}
VerifyBlockingUnaryInterceptor();

async Task<EchoResponse> Call(string target, Interceptor interceptor)
{
    using var channel = GrpcChannel.ForAddress("http://" + target);
    var invoker = channel.Intercept(interceptor);
    var client = new EchoService.EchoServiceClient(invoker);
    return await client.EchoAsync(new EchoRequest { Text = "hello", RequestId = "demo" }, deadline: DateTime.UtcNow.AddSeconds(90));
}

async Task CallStreams(string target)
{
    using var channel = GrpcChannel.ForAddress("http://" + target);
    var client = new EchoService.EchoServiceClient(channel.Intercept(EchoContract.ClientInterceptor()));
    var request = new EchoRequest { Text = "stream", RequestId = "demo" };
    using (var call = client.EchoClientStream(deadline: DateTime.UtcNow.AddSeconds(10)))
    {
        await call.RequestStream.WriteAsync(request);
        await call.RequestStream.CompleteAsync();
        if ((await call.ResponseAsync).Text != "stream") throw new Exception("client-stream response mismatch");
    }
    using (var call = client.EchoServerStream(request, deadline: DateTime.UtcNow.AddSeconds(10)))
    {
        if (!await call.ResponseStream.MoveNext() || call.ResponseStream.Current.Text != "stream") throw new Exception("server-stream response mismatch");
    }
    using (var call = client.EchoDuplex(deadline: DateTime.UtcNow.AddSeconds(10)))
    {
        await call.RequestStream.WriteAsync(request);
        await call.RequestStream.CompleteAsync();
        if (!await call.ResponseStream.MoveNext() || call.ResponseStream.Current.Text != "stream") throw new Exception("duplex response mismatch");
    }
}

async Task ExpectStreamingRejected(string target, Interceptor? interceptor)
{
    using var channel = GrpcChannel.ForAddress("http://" + target);
    CallInvoker invoker = interceptor is null ? channel.CreateCallInvoker() : channel.Intercept(interceptor);
    var client = new EchoService.EchoServiceClient(invoker);
    using var call = client.EchoServerStream(new EchoRequest { Text = "must-not-run" }, deadline: DateTime.UtcNow.AddSeconds(5));
    try
    {
        await call.ResponseStream.MoveNext();
        throw new Exception("server accepted streaming request with missing or wrong contract");
    }
    catch (RpcException error) when (error.StatusCode == StatusCode.FailedPrecondition) { }
}

async Task VerifyDotnetStrictCoverage(string target)
{
    using var channel = GrpcChannel.ForAddress("http://" + target);
    var client = new UnprotectedService.UnprotectedServiceClient(channel.Intercept(new ContractClientInterceptor("demo.echo", "1.1.0")));
    try
    {
        await client.CallAsync(new EchoRequest { Text = "must-not-run" });
        throw new Exception(".NET server dispatched an unprotected service");
    }
    catch (RpcException error) when (error.StatusCode == StatusCode.FailedPrecondition) { }
}

foreach (var (expected,target) in servers)
{
    var response = await Call(target, EchoContract.ClientInterceptor());
    if (response.Text != "hello" || response.ServerLanguage != expected) throw new Exception($"Unexpected response from {expected}");
    await CallStreams(target);
    await ExpectStreamingRejected(target, new ContractClientInterceptor("demo.echo", "1.2.0", EchoContract.Service));
    await ExpectStreamingRejected(target, null);
    foreach (var rejected in new[]{"1.2.0","2.0.0"}) try { await Call(target, new ContractClientInterceptor("demo.echo", rejected)); throw new Exception($"{expected} accepted {rejected}"); }
    catch (RpcException error) when (error.StatusCode == StatusCode.FailedPrecondition) { }
    Console.WriteLine($"PASS .NET client -> {expected} server");
    if (expected == ".NET") await VerifyDotnetStrictCoverage(target);
}
Console.WriteLine(".NET client matrix passed");
