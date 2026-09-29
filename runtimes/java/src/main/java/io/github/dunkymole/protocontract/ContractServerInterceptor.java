package io.github.dunkymole.protocontract;

import io.grpc.*;
import java.util.regex.Pattern;

public final class ContractServerInterceptor implements ServerInterceptor {
  public static final Metadata.Key<String> KEY = Metadata.Key.of("x-proto-contract", Metadata.ASCII_STRING_MARSHALLER);
  private final String api;
  private final Version server;
  final String service;

  public ContractServerInterceptor(String api, String version) {
    this(api, version, null);
  }

  public ContractServerInterceptor(String api, String version, String service) {
    validateConfiguration(api, version);
    this.api = api;
    this.server = Version.parse(version);
    if (service != null && !service.matches("[A-Za-z_][A-Za-z0-9_]*(\\.[A-Za-z_][A-Za-z0-9_]*)*")) throw new IllegalArgumentException("invalid protobuf service name");
    this.service = service;
  }

  @Override public <ReqT, RespT> ServerCall.Listener<ReqT> interceptCall(
      ServerCall<ReqT, RespT> call, Metadata headers, ServerCallHandler<ReqT, RespT> next) {
    String fullName = call.getMethodDescriptor().getFullMethodName();
    if (service != null && !fullName.startsWith(service + "/")) return next.startCall(call, headers);
    Iterable<String> all = headers.getAll(KEY);
    java.util.List<String> values = new java.util.ArrayList<>();
    if (all != null) for (String value : all) values.add(value);
    String error = compatibilityError(api, server.toString(), values);
    if (error != null) {
      call.close(Status.FAILED_PRECONDITION.withDescription(error), new Metadata());
      return new ServerCall.Listener<>() {};
    }
    return next.startCall(call, headers);
  }

  public static String compatibilityError(String api, String serverVersion, java.util.List<String> values) {
    validateConfiguration(api, serverVersion);
    Version server = Version.parse(serverVersion);
    if (values.isEmpty()) return "missing x-proto-contract metadata";
    if (values.size() != 1) return "duplicate x-proto-contract metadata";
    String offered = values.get(0);
    if (offered.length() > 128 || !isAscii(offered)) return "invalid x-proto-contract metadata";
    String prefix = api + "@";
    if (!offered.startsWith(prefix)) return "expected contract API@MAJOR.MINOR.PATCH";
    try {
      Version client = Version.parse(offered.substring(prefix.length()));
      if (client.major != server.major || client.minor > server.minor)
        return "incompatible contract: client " + client + ", server " + server;
      return null;
    } catch (IllegalArgumentException e) { return "invalid semantic version " + offered.substring(prefix.length()); }
  }

  static void validateConfiguration(String api, String version) {
    if (api == null || !Pattern.matches("[A-Za-z0-9._:/-]{1,64}", api)) throw new IllegalArgumentException("invalid contract API identifier");
    Version.parse(version);
    if ((api + "@" + version).length() > 128) throw new IllegalArgumentException("contract metadata exceeds 128 ASCII bytes");
  }
  private static boolean isAscii(String value) { for (int i=0; i<value.length(); i++) if (value.charAt(i) > 0x7f) return false; return true; }

  private record Version(int major, int minor, int patch) {
    static Version parse(String value) {
      String[] p = value.split("\\.", -1);
      if (p.length != 3) throw new IllegalArgumentException("invalid semantic version " + value);
      if (!p[0].matches("0|[1-9][0-9]*") || !p[1].matches("0|[1-9][0-9]*") || !p[2].matches("0|[1-9][0-9]*")) throw new IllegalArgumentException("invalid semantic version " + value);
      try {
        int major = Integer.parseInt(p[0]), minor = Integer.parseInt(p[1]), patch = Integer.parseInt(p[2]);
        return new Version(major, minor, patch);
      }
      catch (NumberFormatException e) { throw new IllegalArgumentException("invalid semantic version " + value); }
    }
    @Override public String toString() { return major + "." + minor + "." + patch; }
  }
}
