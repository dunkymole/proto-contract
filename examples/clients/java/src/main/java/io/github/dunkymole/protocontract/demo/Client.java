package io.github.dunkymole.protocontract.demo;

import io.grpc.*;
import protocontract.generated.echo.Contract;
import io.github.dunkymole.protocontract.ContractClientInterceptor;
import io.github.dunkymole.protocontract.ContractServerInterceptor;
import io.grpc.stub.StreamObserver;
import io.github.dunkymole.protocontract.demo.v1.*;
import java.util.Map;
import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.nio.file.Files;
import java.nio.file.Path;
import com.google.gson.JsonArray;
import com.google.gson.JsonElement;
import com.google.gson.JsonObject;
import com.google.gson.JsonParser;

public final class Client {
  private static void verifyContractVectors() {
    try {
      JsonArray records = JsonParser.parseString(Files.readString(Path.of("/contract-vectors.json"))).getAsJsonArray();
      boolean verifiedTrailingLf = false;
      for (JsonElement record : records) {
        JsonObject vector = record.getAsJsonObject();
        String name = vector.get("name").getAsString();
        String api = vector.get("api").getAsString();
        String server = vector.get("server").getAsString();
        List<String> values = new ArrayList<>();
        for (JsonElement item : vector.getAsJsonArray("values")) values.add(item.getAsString());
        boolean expected = vector.get("compatible").getAsBoolean();
        if (name.equals("trailing-lf")) {
          if (values.size() != 1 || !values.get(0).endsWith("\n")) throw new AssertionError("trailing-lf vector was not JSON-decoded as LF");
          verifiedTrailingLf = true;
        }
        boolean compatible;
        try { compatible = ContractServerInterceptor.compatibilityError(api, server, values) == null; }
        catch (IllegalArgumentException invalidConfiguration) { compatible = false; }
        if (compatible != expected) throw new AssertionError("contract vector failed: " + name);
      }
      if (records.size() != 29 || !verifiedTrailingLf) throw new AssertionError("expected all 29 contract vectors including decoded trailing LF");
    } catch (java.io.IOException error) { throw new RuntimeException(error); }
  }

  private static final class Collector implements StreamObserver<EchoResponse> {
    final List<EchoResponse> values = new ArrayList<>();
    final CountDownLatch done = new CountDownLatch(1);
    Throwable error;
    public void onNext(EchoResponse value) { values.add(value); }
    public void onError(Throwable value) { error = value; done.countDown(); }
    public void onCompleted() { done.countDown(); }
    List<EchoResponse> await() {
      try { if (!done.await(15, TimeUnit.SECONDS)) throw new AssertionError("stream timed out"); }
      catch (InterruptedException e) { Thread.currentThread().interrupt(); throw new RuntimeException(e); }
      if (error != null) throw new RuntimeException(error);
      return values;
    }
  }

  private static void callStreams(String target, ClientInterceptor interceptor) {
    ManagedChannel channel = ManagedChannelBuilder.forTarget("dns:///" + target).usePlaintext().build();
    try {
      EchoRequest request = EchoRequest.newBuilder().setText("stream").setRequestId("demo").build();
      EchoServiceGrpc.EchoServiceStub stub = EchoServiceGrpc.newStub(channel).withInterceptors(interceptor).withDeadlineAfter(15, TimeUnit.SECONDS);
      Collector clientResponse = new Collector();
      StreamObserver<EchoRequest> clientInput = stub.echoClientStream(clientResponse);
      clientInput.onNext(request); clientInput.onCompleted();
      if (!clientResponse.await().get(0).getText().equals("stream")) throw new AssertionError("client-stream response mismatch");
      Collector serverResponse = new Collector();
      stub.echoServerStream(request, serverResponse);
      if (!serverResponse.await().get(0).getText().equals("stream")) throw new AssertionError("server-stream response mismatch");
      Collector duplexResponse = new Collector();
      StreamObserver<EchoRequest> duplexInput = stub.echoDuplex(duplexResponse);
      duplexInput.onNext(request); duplexInput.onCompleted();
      if (!duplexResponse.await().get(0).getText().equals("stream")) throw new AssertionError("duplex response mismatch");
    } finally { channel.shutdownNow(); }
  }

  private static EchoResponse call(String target, ClientInterceptor interceptor) {
    ManagedChannel channel = ManagedChannelBuilder.forTarget("dns:///" + target).usePlaintext().build();
    try {
      EchoServiceGrpc.EchoServiceBlockingStub stub = EchoServiceGrpc.newBlockingStub(channel)
          .withInterceptors(interceptor).withDeadlineAfter(90, TimeUnit.SECONDS);
      return stub.echo(EchoRequest.newBuilder().setText("hello").setRequestId("demo").build());
    } finally { channel.shutdownNow(); }
  }
  private static void verifyJavaStrictCoverage(String target) {
    ManagedChannel channel = ManagedChannelBuilder.forTarget("dns:///" + target).usePlaintext().build();
    try {
      var stub = UnprotectedServiceGrpc.newBlockingStub(channel)
          .withInterceptors(new ContractClientInterceptor("demo.echo", "1.1.0"));
      try {
        stub.call(EchoRequest.newBuilder().setText("must-not-run").build());
        throw new AssertionError("Java server dispatched an unprotected service");
      } catch (StatusRuntimeException error) {
        if (error.getStatus().getCode() != Status.Code.FAILED_PRECONDITION) throw error;
      }
    } finally { channel.shutdownNow(); }
  }
  public static void main(String[] args) {
    verifyContractVectors();
    Map<String,String> servers = Map.of("Go","go-server:50051","Java","java-server:50053",".NET","dotnet-server:50054","Python","python-server:50055");
    servers.forEach((expected,target) -> {
      EchoResponse response = call(target, Contract.clientInterceptor());
      if (!response.getText().equals("hello") || !response.getServerLanguage().equals(expected)) throw new AssertionError(response);
      callStreams(target, Contract.clientInterceptor());
      for (String rejected : new String[]{"1.2.0","2.0.0"}) try { call(target, new ContractClientInterceptor("demo.echo", rejected)); throw new AssertionError(expected+" accepted "+rejected); }
      catch (StatusRuntimeException error) { if (error.getStatus().getCode()!=Status.Code.FAILED_PRECONDITION) throw error; }
      System.out.println("PASS Java client -> "+expected+" server");
      if (expected.equals("Java")) verifyJavaStrictCoverage(target);
    });
    System.out.println("Java client matrix passed");
  }
}
