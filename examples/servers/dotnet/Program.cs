using EchoContract = ProtoContract.Generated.Echo.Contract;
using ProtoContract.Demo.V1;
using Grpc.Core;

var builder = WebApplication.CreateBuilder(args);
builder.WebHost.ConfigureKestrel(options => options.ListenAnyIP(50054, listen => listen.Protocols = Microsoft.AspNetCore.Server.Kestrel.Core.HttpProtocols.Http2));
builder.Services.AddSingleton(EchoContract.StrictServerInterceptor());
builder.Services.AddGrpc(options => options.Interceptors.Add<ProtoContract.StrictServerInterceptor>());
var app = builder.Build();
app.MapGrpcService<EchoServiceImpl>();
app.MapGrpcService<UnprotectedServiceImpl>();
app.Run();

sealed class EchoServiceImpl : EchoService.EchoServiceBase
{
    public override Task<EchoResponse> Echo(EchoRequest request, ServerCallContext context) =>
        Task.FromResult(new EchoResponse { Text = request.Text, ServerLanguage = ".NET" });

    public override async Task<EchoResponse> EchoClientStream(IAsyncStreamReader<EchoRequest> requestStream, ServerCallContext context)
    {
        var values = new List<string>();
        while (await requestStream.MoveNext(context.CancellationToken)) values.Add(requestStream.Current.Text);
        return new EchoResponse { Text = string.Join(",", values), ServerLanguage = ".NET" };
    }

    public override async Task EchoServerStream(EchoRequest request, IServerStreamWriter<EchoResponse> responseStream, ServerCallContext context) =>
        await responseStream.WriteAsync(new EchoResponse { Text = request.Text, ServerLanguage = ".NET" });

    public override async Task EchoDuplex(IAsyncStreamReader<EchoRequest> requestStream, IServerStreamWriter<EchoResponse> responseStream, ServerCallContext context)
    {
        while (await requestStream.MoveNext(context.CancellationToken))
            await responseStream.WriteAsync(new EchoResponse { Text = requestStream.Current.Text, ServerLanguage = ".NET" });
    }
}

sealed class UnprotectedServiceImpl : UnprotectedService.UnprotectedServiceBase
{
    public override Task<EchoResponse> Call(EchoRequest request, ServerCallContext context) =>
        throw new InvalidOperationException("strict server dispatched an unprotected service");
}
