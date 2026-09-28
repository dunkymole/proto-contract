import type { Interceptor } from "@connectrpc/connect";

export const metadataKey = "x-proto-contract";

/** Apply per generated client using grpc-bridge's public interceptTransport(). */
export function contractClientInterceptor(api: string, version: string): Interceptor {
  const value = `${api}@${version}`;
  return (next) => async (request) => {
    request.header.set(metadataKey, value);
    return next(request);
  };
}
