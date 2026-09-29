package io.github.dunkymole.protocontract;

import io.grpc.*;
import java.util.*;
import java.util.regex.Pattern;

/** Owned fail-closed service dispatch. Every service needs a contract or exemption. */
public final class StrictServerInterceptor implements ServerInterceptor {
  private static final Pattern SERVICE = Pattern.compile("[A-Za-z_][A-Za-z0-9_]*(\\.[A-Za-z_][A-Za-z0-9_]*)*");
  private final Map<String, ContractServerInterceptor> contracts;
  private final Set<String> exemptions;

  public StrictServerInterceptor(Map<String, ContractServerInterceptor> contracts, Set<String> exemptions) {
    this.contracts = Map.copyOf(contracts);
    this.exemptions = Set.copyOf(exemptions);
    for (var entry : this.contracts.entrySet()) {
      String service = entry.getKey();
      if (!SERVICE.matcher(service).matches() || !service.equals(entry.getValue().service))
        throw new IllegalArgumentException("contract interceptor is not bound to service " + service);
      if (this.exemptions.contains(service)) throw new IllegalArgumentException("service cannot be both protected and exempt");
    }
    for (String service : this.exemptions) if (!SERVICE.matcher(service).matches()) throw new IllegalArgumentException("invalid protobuf service name " + service);
  }

  @Override public <ReqT, RespT> ServerCall.Listener<ReqT> interceptCall(
      ServerCall<ReqT, RespT> call, Metadata headers, ServerCallHandler<ReqT, RespT> next) {
    String full = call.getMethodDescriptor().getFullMethodName();
    int slash = full.lastIndexOf('/');
    String service = slash < 0 ? "" : full.substring(0, slash);
    ContractServerInterceptor interceptor = contracts.get(service);
    if (interceptor != null) return interceptor.interceptCall(call, headers, next);
    if (exemptions.contains(service)) return next.startCall(call, headers);
    call.close(Status.FAILED_PRECONDITION.withDescription("unprotected service registration"), new Metadata());
    return new ServerCall.Listener<>() {};
  }
}
