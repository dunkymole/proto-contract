using Grpc.Core;
using Grpc.Core.Interceptors;

namespace ProtoContract;

public class ContractServerInterceptor(string api, string serverVersion) : Interceptor
{
    public override Task<TResponse> UnaryServerHandler<TRequest, TResponse>(TRequest request, ServerCallContext context, UnaryServerMethod<TRequest, TResponse> continuation)
    {
        var offered = context.RequestHeaders.GetValue("x-proto-contract")
            ?? throw new RpcException(new Status(StatusCode.FailedPrecondition, "missing x-proto-contract metadata"));
        if (!Compatible(api, offered, serverVersion, out var error))
            throw new RpcException(new Status(StatusCode.FailedPrecondition, error));
        return continuation(request, context);
    }

    public static bool Compatible(string api, string offered, string serverVersion, out string error)
    {
        var prefix = api + "@";
        if (!offered.StartsWith(prefix, StringComparison.Ordinal) || !Version.TryParse(offered[prefix.Length..], out var client) || !Version.TryParse(serverVersion, out var server))
        { error = $"expected contract {prefix}MAJOR.MINOR.PATCH"; return false; }
        if (client.Major != server.Major || client.Minor > server.Minor)
        { error = $"incompatible contract: client {client}, server {server}"; return false; }
        error = ""; return true;
    }
}
