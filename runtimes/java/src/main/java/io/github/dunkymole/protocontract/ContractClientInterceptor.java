package io.github.dunkymole.protocontract;

import io.grpc.*;

public final class ContractClientInterceptor implements ClientInterceptor {
  private final String value;

  public ContractClientInterceptor(String api, String version) { this.value = api + "@" + version; }

  @Override public <ReqT, RespT> ClientCall<ReqT, RespT> interceptCall(
      MethodDescriptor<ReqT, RespT> method, CallOptions options, Channel next) {
    return new ForwardingClientCall.SimpleForwardingClientCall<>(next.newCall(method, options)) {
      @Override public void start(Listener<RespT> responseListener, Metadata headers) {
        headers.put(ContractServerInterceptor.KEY, value);
        super.start(responseListener, headers);
      }
    };
  }
}
