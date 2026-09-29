using System.Text.RegularExpressions;
using Grpc.Core;
using Grpc.Core.Interceptors;

namespace ProtoContract;

public sealed class ContractClientInterceptor : Interceptor
{
    private readonly string value;
    private readonly string? service;

    public ContractClientInterceptor(string api, string clientVersion, string? service = null)
    {
        ContractServerInterceptor.ValidateConfiguration(api, clientVersion);
        if (service is not null && !Regex.IsMatch(service, "\\A[A-Za-z_][A-Za-z0-9_]*(\\.[A-Za-z_][A-Za-z0-9_]*)*\\z", RegexOptions.CultureInvariant)) throw new ArgumentException("invalid protobuf service name");
        value = $"{api}@{clientVersion}";
        this.service = service;
    }

    private ClientInterceptorContext<TRequest, TResponse> Stamp<TRequest, TResponse>(ClientInterceptorContext<TRequest, TResponse> context)
        where TRequest : class where TResponse : class
    {
        var name = context.Method.FullName;
        if (service is not null && !name.StartsWith("/" + service + "/", StringComparison.Ordinal)) return context;
        var headers = new Metadata();
        if (context.Options.Headers is not null)
            foreach (var entry in context.Options.Headers)
                if (entry.Key != ContractServerInterceptor.MetadataKey)
                    if (entry.IsBinary) headers.Add(entry.Key, entry.ValueBytes); else headers.Add(entry.Key, entry.Value);
        headers.Add(ContractServerInterceptor.MetadataKey, value);
        return new ClientInterceptorContext<TRequest, TResponse>(context.Method, context.Host, context.Options.WithHeaders(headers));
    }

    public override AsyncUnaryCall<TResponse> AsyncUnaryCall<TRequest, TResponse>(TRequest request, ClientInterceptorContext<TRequest, TResponse> context, AsyncUnaryCallContinuation<TRequest, TResponse> continuation) => continuation(request, Stamp(context));
    public override AsyncServerStreamingCall<TResponse> AsyncServerStreamingCall<TRequest, TResponse>(TRequest request, ClientInterceptorContext<TRequest, TResponse> context, AsyncServerStreamingCallContinuation<TRequest, TResponse> continuation) => continuation(request, Stamp(context));
    public override AsyncClientStreamingCall<TRequest, TResponse> AsyncClientStreamingCall<TRequest, TResponse>(ClientInterceptorContext<TRequest, TResponse> context, AsyncClientStreamingCallContinuation<TRequest, TResponse> continuation) => continuation(Stamp(context));
    public override AsyncDuplexStreamingCall<TRequest, TResponse> AsyncDuplexStreamingCall<TRequest, TResponse>(ClientInterceptorContext<TRequest, TResponse> context, AsyncDuplexStreamingCallContinuation<TRequest, TResponse> continuation) => continuation(Stamp(context));
    public override TResponse BlockingUnaryCall<TRequest, TResponse>(TRequest request, ClientInterceptorContext<TRequest, TResponse> context, BlockingUnaryCallContinuation<TRequest, TResponse> continuation) => continuation(request, Stamp(context));
}
