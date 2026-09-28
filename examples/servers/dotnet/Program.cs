using EchoContract = ProtoContract.Generated.Echo.Contract;
using ProtoContract.Demo.V1;
using Grpc.Core;

var builder = WebApplication.CreateBuilder(args);
builder.WebHost.ConfigureKestrel(options => options.ListenAnyIP(50054, listen => listen.Protocols = Microsoft.AspNetCore.Server.Kestrel.Core.HttpProtocols.Http2));
builder.Services.AddGrpc().AddServiceOptions<EchoServiceImpl>(options =>
    options.Interceptors.Add<EchoContract.ServerInterceptor>());
var app = builder.Build();
app.MapGrpcService<EchoServiceImpl>();
app.Run();

sealed class EchoServiceImpl : EchoService.EchoServiceBase
{
    public override Task<EchoResponse> Echo(EchoRequest request, ServerCallContext context) =>
        Task.FromResult(new EchoResponse { Text = request.Text, ServerLanguage = ".NET" });
}
