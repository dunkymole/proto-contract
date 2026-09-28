import type { Interceptor, Transport } from "@connectrpc/connect";

export const metadataKey = "x-proto-contract";

/** A Connect interceptor for transports dedicated to one service contract. */
export function contractClientInterceptor(api: string, version: string): Interceptor {
  const value = `${api}@${version}`;
  return (next) => async (request) => {
    // Replace a caller-supplied value: servers require exactly one contract.
    request.header.set(metadataKey, value);
    return next(request);
  };
}

/** Bind a service client's contract without changing the shared bridge transport. */
export function contractClientTransport(transport: Transport, api: string, version: string): Transport {
  const withContract = (headers: HeadersInit | undefined) => {
    const result = new Headers(headers);
    result.set(metadataKey, `${api}@${version}`);
    return result;
  };
  return {
    unary(method, signal, timeoutMs, header, message, contextValues) {
      return transport.unary(method, signal, timeoutMs, withContract(header), message, contextValues);
    },
    stream(method, signal, timeoutMs, header, message, contextValues) {
      return transport.stream(method, signal, timeoutMs, withContract(header), message, contextValues);
    },
  };
}
