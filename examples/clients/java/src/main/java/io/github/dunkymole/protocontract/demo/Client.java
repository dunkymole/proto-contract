package io.github.dunkymole.protocontract.demo;

import io.grpc.*;
import protocontract.generated.echo.Contract;
import io.github.dunkymole.protocontract.ContractClientInterceptor;
import io.github.dunkymole.protocontract.demo.v1.*;
import java.util.Map;
import java.util.concurrent.TimeUnit;

public final class Client {
  private static EchoResponse call(String target, ClientInterceptor interceptor) {
    ManagedChannel channel = ManagedChannelBuilder.forTarget("dns:///" + target).usePlaintext().build();
    try {
      EchoServiceGrpc.EchoServiceBlockingStub stub = EchoServiceGrpc.newBlockingStub(channel)
          .withInterceptors(interceptor).withDeadlineAfter(90, TimeUnit.SECONDS);
      return stub.echo(EchoRequest.newBuilder().setText("hello").setRequestId("demo").build());
    } finally { channel.shutdownNow(); }
  }
  public static void main(String[] args) {
    Map<String,String> servers = Map.of("Go","go-server:50051","Java","java-server:50053",".NET","dotnet-server:50054","Python","python-server:50055");
    servers.forEach((expected,target) -> {
      EchoResponse response = call(target, Contract.clientInterceptor());
      if (!response.getText().equals("hello") || !response.getServerLanguage().equals(expected)) throw new AssertionError(response);
      for (String rejected : new String[]{"1.1.0","2.1.0"}) try { call(target, new ContractClientInterceptor("demo.echo", rejected)); throw new AssertionError(expected+" accepted "+rejected); }
      catch (StatusRuntimeException error) { if (error.getStatus().getCode()!=Status.Code.FAILED_PRECONDITION) throw error; }
      System.out.println("PASS Java client -> "+expected+" server");
    });
    System.out.println("Java client matrix passed");
  }
}
