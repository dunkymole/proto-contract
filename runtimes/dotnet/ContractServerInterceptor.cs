using System.Text.RegularExpressions;
using Grpc.Core;
using Grpc.Core.Interceptors;

namespace ProtoContract;

public class ContractServerInterceptor : Interceptor
{
    public const string MetadataKey = "x-proto-contract";
    private static readonly Regex ApiPattern = new("\\A[A-Za-z0-9._:/-]{1,64}\\z", RegexOptions.CultureInvariant);
    private static readonly Regex ServicePattern = new("\\A[A-Za-z_][A-Za-z0-9_]*(\\.[A-Za-z_][A-Za-z0-9_]*)*\\z", RegexOptions.CultureInvariant);
    private static readonly Regex VersionPattern = new("\\A(0|[1-9][0-9]*)\\.(0|[1-9][0-9]*)\\.(0|[1-9][0-9]*)\\z", RegexOptions.CultureInvariant);
    private readonly string api;
    private readonly string serverVersion;
    internal readonly string? Service;

    public ContractServerInterceptor(string api, string serverVersion, string? service = null)
    {
        ValidateConfiguration(api, serverVersion);
        this.api = api;
        this.serverVersion = serverVersion;
        if (service is not null && !ServicePattern.IsMatch(service)) throw new ArgumentException("invalid protobuf service name");
        Service = service;
    }

    public static void ValidateConfiguration(string api, string version)
    {
        if (api is null || !ApiPattern.IsMatch(api)) throw new ArgumentException("invalid contract API identifier");
        if (!TryParse(version, out _)) throw new ArgumentException($"invalid semantic version {version}");
        if (System.Text.Encoding.ASCII.GetByteCount(api + "@" + version) > 128) throw new ArgumentException("contract metadata exceeds 128 ASCII bytes");
    }

    private bool Applies(ServerCallContext context) => Service is null || context.Method.StartsWith("/" + Service + "/", StringComparison.Ordinal);
    internal Status? Validate(ServerCallContext context)
    {
        if (!Applies(context)) return null;
        var values = context.RequestHeaders.Where(entry => entry.Key == MetadataKey).ToArray();
        var error = CompatibilityError(api, serverVersion, values.Select(entry => entry.Value).ToArray());
        return error is null ? null : new Status(StatusCode.FailedPrecondition, error);
    }

    public static string? CompatibilityError(string api, string serverVersion, IReadOnlyList<string> offeredValues)
    {
        ValidateConfiguration(api, serverVersion);
        _ = TryParse(serverVersion, out var server);
        if (offeredValues.Count == 0) return "missing x-proto-contract metadata";
        if (offeredValues.Count != 1) return "duplicate x-proto-contract metadata";
        var offered = offeredValues[0];
        if (offered is null || offered.Any(ch => ch > 127) || System.Text.Encoding.ASCII.GetByteCount(offered) > 128)
            return "invalid x-proto-contract metadata";
        var at = offered.IndexOf('@');
        if (at < 0 || !ApiPattern.IsMatch(offered[..at]) || !StringComparer.Ordinal.Equals(offered[..at], api))
            return "expected contract API@MAJOR.MINOR.PATCH";
        var clientVersion = offered[(at + 1)..];
        if (!TryParse(clientVersion, out var client)) return $"invalid semantic version {clientVersion}";
        if (client.Major != server.Major || client.Minor > server.Minor)
            return $"incompatible contract: client {clientVersion}, server {serverVersion}";
        return null;
    }

    public static bool Compatible(string api, string offered, string version, out string error)
    {
        error = CompatibilityError(api, version, new[] { offered }) ?? "";
        return error.Length == 0;
    }

    private static bool TryParse(string? value, out Version version)
    {
        version = new Version(0, 0, 0);
        var match = value is null ? Match.Empty : VersionPattern.Match(value);
        if (!match.Success) return false;
        var parts = new int[3];
        for (var i = 0; i < parts.Length; i++)
            if (!int.TryParse(match.Groups[i + 1].Value, System.Globalization.NumberStyles.None, System.Globalization.CultureInfo.InvariantCulture, out parts[i])) return false;
        version = new Version(parts[0], parts[1], parts[2]);
        return true;
    }

    public override Task<TResponse> UnaryServerHandler<TRequest, TResponse>(TRequest request, ServerCallContext context, UnaryServerMethod<TRequest, TResponse> continuation)
        => Validate(context) is { } error ? Task.FromException<TResponse>(new RpcException(error)) : continuation(request, context);
    public override Task<TResponse> ClientStreamingServerHandler<TRequest, TResponse>(IAsyncStreamReader<TRequest> requestStream, ServerCallContext context, ClientStreamingServerMethod<TRequest, TResponse> continuation)
        => Validate(context) is { } error ? Task.FromException<TResponse>(new RpcException(error)) : continuation(requestStream, context);
    public override Task ServerStreamingServerHandler<TRequest, TResponse>(TRequest request, IServerStreamWriter<TResponse> responseStream, ServerCallContext context, ServerStreamingServerMethod<TRequest, TResponse> continuation)
        => Validate(context) is { } error ? Task.FromException(new RpcException(error)) : continuation(request, responseStream, context);
    public override Task DuplexStreamingServerHandler<TRequest, TResponse>(IAsyncStreamReader<TRequest> requestStream, IServerStreamWriter<TResponse> responseStream, ServerCallContext context, DuplexStreamingServerMethod<TRequest, TResponse> continuation)
        => Validate(context) is { } error ? Task.FromException(new RpcException(error)) : continuation(requestStream, responseStream, context);

    private readonly record struct Version(int Major, int Minor, int Patch);
}
