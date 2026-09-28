using ProtoContract;
using ProtoContract.Demo.V1;
using Grpc.Core;

var builder = WebApplication.CreateBuilder(args);
builder.WebHost.ConfigureKestrel(options => options.ListenAnyIP(50054, listen => listen.Protocols = Microsoft.AspNetCore.Server.Kestrel.Core.HttpProtocols.Http2));
builder.Services.AddGrpc(options => options.Interceptors.Add<ContractServerInterceptor>());
builder.Services.AddSingleton(new ContractServerInterceptor("demo.echo", "1.1.0"));
var app = builder.Build();
app.MapGrpcService<EchoServiceImpl>();
app.Run();

sealed class EchoServiceImpl : EchoService.EchoServiceBase
{
    public override Task<EchoResponse> Echo(EchoRequest request, ServerCallContext context) =>
        Task.FromResult(new EchoResponse { Text = request.Text, ServerLanguage = ".NET" });
}
