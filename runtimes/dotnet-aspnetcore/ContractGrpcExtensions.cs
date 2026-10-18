using Grpc.AspNetCore.Server;
using Microsoft.AspNetCore.Builder;
using Microsoft.AspNetCore.Routing;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Hosting;

namespace ProtoContract;

public static class ContractGrpcExtensions
{
    // Install once. Endpoint helpers contribute contracts before the host starts.
    public static IServiceCollection AddContractGrpc(this IServiceCollection services, params string[] exemptions)
    {
        if (services.Any(d => d.ServiceType == typeof(ContractRegistry)))
            throw new InvalidOperationException("AddContractGrpc already called");
        services.AddSingleton(new ContractRegistry(exemptions.ToArray()));
        services.AddSingleton(sp => sp.GetRequiredService<ContractRegistry>().Freeze());
        services.AddHostedService<FreezeContracts>();
        services.AddGrpc(options => options.Interceptors.Add<StrictServerInterceptor>());
        return services;
    }

    public static GrpcServiceEndpointConventionBuilder MapContractGrpcService<TService>(
        this IEndpointRouteBuilder endpoints, ContractServerInterceptor contract) where TService : class
    {
        var registry = endpoints.ServiceProvider.GetRequiredService<ContractRegistry>();
        registry.Add(contract);
        var mapped = endpoints.MapGrpcService<TService>();
        mapped.Add(endpoint =>
        {
            var metadata = endpoint.Metadata.OfType<GrpcMethodMetadata>().FirstOrDefault();
            if (metadata is not null && metadata.Method.ServiceName != contract.ServiceName)
                throw new InvalidOperationException("registration does not match contract service " + contract.ServiceName);
        });
        return mapped;
    }

    private sealed class FreezeContracts(ContractRegistry registry) : IHostedService
    {
        public Task StartAsync(CancellationToken cancellationToken) { registry.Freeze(); return Task.CompletedTask; }
        public Task StopAsync(CancellationToken cancellationToken) => Task.CompletedTask;
    }

    private sealed class ContractRegistry(string[] exemptions)
    {
        private readonly Dictionary<string, ContractServerInterceptor> contracts = new(StringComparer.Ordinal);
        private StrictServerInterceptor? strict;
        public void Add(ContractServerInterceptor contract)
        {
            if (strict is not null) throw new InvalidOperationException("contract registrations are frozen");
            if (contract.ServiceName is null) throw new ArgumentException("service-scoped contract required");
            if (exemptions.Contains(contract.ServiceName, StringComparer.Ordinal)) throw new ArgumentException("service cannot be protected and exempt");
            if (!contracts.TryAdd(contract.ServiceName, contract)) throw new ArgumentException("duplicate service " + contract.ServiceName);
        }
        public StrictServerInterceptor Freeze() => strict ??= new StrictServerInterceptor(contracts, exemptions);
    }
}
