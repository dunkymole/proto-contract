using Grpc.Core;
using Grpc.Core.Interceptors;

namespace ProtoContract;

public sealed class ContractClientInterceptor(string api, string clientVersion) : Interceptor
{
    private readonly string value = $"{api}@{clientVersion}";

    public override AsyncUnaryCall<TResponse> AsyncUnaryCall<TRequest, TResponse>(
        TRequest request,
        ClientInterceptorContext<TRequest, TResponse> context,
        AsyncUnaryCallContinuation<TRequest, TResponse> continuation)
    {
        var headers = new Metadata();
        if (context.Options.Headers is not null)
            foreach (var entry in context.Options.Headers)
                if (entry.IsBinary) headers.Add(entry.Key, entry.ValueBytes); else headers.Add(entry.Key, entry.Value);
        headers.Add("x-proto-contract", value);
        var next = new ClientInterceptorContext<TRequest, TResponse>(context.Method, context.Host, context.Options.WithHeaders(headers));
        return continuation(request, next);
    }
}
