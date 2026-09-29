using System.Text.RegularExpressions;
using Grpc.Core;
using Grpc.Core.Interceptors;

namespace ProtoContract;

/// <summary>Owned fail-closed service dispatch; every service needs a contract or exemption.</summary>
public sealed class StrictServerInterceptor : Interceptor
{
    private readonly IReadOnlyDictionary<string, ContractServerInterceptor> contracts;
    private readonly HashSet<string> exemptions;
    private static readonly Regex ServicePattern = new("\\A[A-Za-z_][A-Za-z0-9_]*(\\.[A-Za-z_][A-Za-z0-9_]*)*\\z", RegexOptions.CultureInvariant);

    public StrictServerInterceptor(IReadOnlyDictionary<string, ContractServerInterceptor> contracts, IEnumerable<string>? exemptions = null)
    {
        this.contracts = new Dictionary<string, ContractServerInterceptor>(contracts, StringComparer.Ordinal);
        this.exemptions = new HashSet<string>(exemptions ?? Array.Empty<string>(), StringComparer.Ordinal);
        foreach (var (service, interceptor) in this.contracts)
        {
            if (!ServicePattern.IsMatch(service) || interceptor.Service != service) throw new ArgumentException($"contract interceptor is not bound to service {service}");
            if (this.exemptions.Contains(service)) throw new ArgumentException("service cannot be both protected and exempt");
        }
        if (this.exemptions.Any(service => !ServicePattern.IsMatch(service))) throw new ArgumentException("invalid protobuf service name");
    }

    private Status? ValidateStrict(ServerCallContext context)
    {
        var slash = context.Method.LastIndexOf('/');
        var service = slash < 0 ? "" : context.Method[..slash].TrimStart('/');
        if (contracts.TryGetValue(service, out var interceptor)) return interceptor.Validate(context);
        if (exemptions.Contains(service)) return null;
        return new Status(StatusCode.FailedPrecondition, "unprotected service registration");
    }

    public override Task<TResponse> UnaryServerHandler<TRequest, TResponse>(TRequest request, ServerCallContext context, UnaryServerMethod<TRequest, TResponse> continuation)
        => ValidateStrict(context) is { } error ? Task.FromException<TResponse>(new RpcException(error)) : continuation(request, context);
    public override Task<TResponse> ClientStreamingServerHandler<TRequest, TResponse>(IAsyncStreamReader<TRequest> requestStream, ServerCallContext context, ClientStreamingServerMethod<TRequest, TResponse> continuation)
        => ValidateStrict(context) is { } error ? Task.FromException<TResponse>(new RpcException(error)) : continuation(requestStream, context);
    public override Task ServerStreamingServerHandler<TRequest, TResponse>(TRequest request, IServerStreamWriter<TResponse> responseStream, ServerCallContext context, ServerStreamingServerMethod<TRequest, TResponse> continuation)
        => ValidateStrict(context) is { } error ? Task.FromException(new RpcException(error)) : continuation(request, responseStream, context);
    public override Task DuplexStreamingServerHandler<TRequest, TResponse>(IAsyncStreamReader<TRequest> requestStream, IServerStreamWriter<TResponse> responseStream, ServerCallContext context, DuplexStreamingServerMethod<TRequest, TResponse> continuation)
        => ValidateStrict(context) is { } error ? Task.FromException(new RpcException(error)) : continuation(requestStream, responseStream, context);
}
