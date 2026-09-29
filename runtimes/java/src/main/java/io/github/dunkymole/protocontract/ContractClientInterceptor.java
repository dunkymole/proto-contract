package io.github.dunkymole.protocontract;

import io.grpc.*;

public final class ContractClientInterceptor implements ClientInterceptor {
  private final String value;
  private final String service;

  public ContractClientInterceptor(String api, String version) { this(api, version, null); }
  public ContractClientInterceptor(String api, String version, String service) {
    ContractServerInterceptor.validateConfiguration(api, version);
    if (service != null && !service.matches("[A-Za-z_][A-Za-z0-9_]*(\\.[A-Za-z_][A-Za-z0-9_]*)*")) throw new IllegalArgumentException("invalid protobuf service name");
    this.value = api + "@" + version;
    this.service = service;
  }

  @Override public <ReqT, RespT> ClientCall<ReqT, RespT> interceptCall(
      MethodDescriptor<ReqT, RespT> method, CallOptions options, Channel next) {
    String fullName = method.getFullMethodName();
    if (service != null && !fullName.startsWith(service + "/")) return next.newCall(method, options);
    return new ForwardingClientCall.SimpleForwardingClientCall<>(next.newCall(method, options)) {
      @Override public void start(Listener<RespT> responseListener, Metadata headers) {
        headers.removeAll(ContractServerInterceptor.KEY);
        headers.put(ContractServerInterceptor.KEY, value);
        super.start(responseListener, headers);
      }
    };
  }
}
