package io.github.dunkymole.protocontract;

import io.grpc.*;

public final class ContractServerInterceptor implements ServerInterceptor {
  public static final Metadata.Key<String> KEY = Metadata.Key.of("x-proto-contract", Metadata.ASCII_STRING_MARSHALLER);
  private final String api;
  private final Version server;

  public ContractServerInterceptor(String api, String version) {
    this.api = api;
    this.server = Version.parse(version);
  }

  @Override public <ReqT, RespT> ServerCall.Listener<ReqT> interceptCall(
      ServerCall<ReqT, RespT> call, Metadata headers, ServerCallHandler<ReqT, RespT> next) {
    String offered = headers.get(KEY);
    String error = compatibilityError(offered);
    if (error != null) {
      call.close(Status.FAILED_PRECONDITION.withDescription(error), new Metadata());
      return new ServerCall.Listener<>() {};
    }
    return next.startCall(call, headers);
  }

  private String compatibilityError(String offered) {
    if (offered == null) return "missing x-proto-contract metadata";
    String prefix = api + "@";
    if (!offered.startsWith(prefix)) return "expected contract " + prefix + "MAJOR.MINOR.PATCH";
    try {
      Version client = Version.parse(offered.substring(prefix.length()));
      if (client.major != server.major || client.minor > server.minor)
        return "incompatible contract: client " + client + ", server " + server;
      return null;
    } catch (IllegalArgumentException e) { return e.getMessage(); }
  }

  private record Version(int major, int minor, int patch) {
    static Version parse(String value) {
      String[] p = value.split("\\.", -1);
      if (p.length != 3) throw new IllegalArgumentException("invalid semantic version " + value);
      try { return new Version(Integer.parseInt(p[0]), Integer.parseInt(p[1]), Integer.parseInt(p[2])); }
      catch (NumberFormatException e) { throw new IllegalArgumentException("invalid semantic version " + value); }
    }
    @Override public String toString() { return major + "." + minor + "." + patch; }
  }
}
