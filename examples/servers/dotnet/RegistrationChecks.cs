using Microsoft.AspNetCore.Routing;
using ProtoContract;
using EchoContract = ProtoContract.Generated.Echo.Contract;

// Executed by the interoperability fixture before serving requests.
static class RegistrationChecks
{
    public static async Task Verify()
    {
        var builder = WebApplication.CreateBuilder();
        builder.Logging.ClearProviders();
        builder.Services.AddContractGrpc();
        Reject<InvalidOperationException>(() => builder.Services.AddContractGrpc());
        await using (var app = builder.Build())
        {
            EchoContract.MapService<EchoServiceImpl>(app);
            _ = ((IEndpointRouteBuilder)app).DataSources.SelectMany(source => source.Endpoints).ToArray();
            Reject<ArgumentException>(() => EchoContract.MapService<EchoServiceImpl>(app));
            _ = app.Services.GetRequiredService<StrictServerInterceptor>();
            Reject<InvalidOperationException>(() => EchoContract.MapService<EchoServiceImpl>(app));
        }

        var wrong = WebApplication.CreateBuilder();
        wrong.Logging.ClearProviders();
        wrong.Services.AddContractGrpc();
        await using (var app = wrong.Build())
        {
            app.MapContractGrpcService<UnprotectedServiceImpl>(new EchoContract.ServerInterceptor());
            Reject<InvalidOperationException>(() =>
            {
                _ = ((IEndpointRouteBuilder)app).DataSources.SelectMany(source => source.Endpoints).ToArray();
            });
        }
    }

    private static void Reject<T>(Action action) where T : Exception
    {
        try { action(); }
        catch (T) { return; }
        throw new Exception("combined registration accepted invalid configuration: " + typeof(T).Name);
    }
}
