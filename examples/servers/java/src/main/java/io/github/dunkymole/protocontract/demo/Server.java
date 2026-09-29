package io.github.dunkymole.protocontract.demo;

import io.grpc.ServerBuilder;
import io.grpc.stub.StreamObserver;
import protocontract.generated.echo.Contract;
import io.github.dunkymole.protocontract.demo.v1.*;

public final class Server {
  private static final class EchoImpl extends EchoServiceGrpc.EchoServiceImplBase {
    @Override public void echo(EchoRequest request, StreamObserver<EchoResponse> observer) {
      observer.onNext(EchoResponse.newBuilder().setText(request.getText()).setServerLanguage("Java").build());
      observer.onCompleted();
    }
    @Override public StreamObserver<EchoRequest> echoClientStream(StreamObserver<EchoResponse> observer) {
      return new StreamObserver<>() {
        private final StringBuilder text = new StringBuilder();
        @Override public void onNext(EchoRequest request) { if (text.length() > 0) text.append(','); text.append(request.getText()); }
        @Override public void onError(Throwable error) { observer.onError(error); }
        @Override public void onCompleted() { observer.onNext(EchoResponse.newBuilder().setText(text.toString()).setServerLanguage("Java").build()); observer.onCompleted(); }
      };
    }
    @Override public void echoServerStream(EchoRequest request, StreamObserver<EchoResponse> observer) {
      observer.onNext(EchoResponse.newBuilder().setText(request.getText()).setServerLanguage("Java").build()); observer.onCompleted();
    }
    @Override public StreamObserver<EchoRequest> echoDuplex(StreamObserver<EchoResponse> observer) {
      return new StreamObserver<>() {
        @Override public void onNext(EchoRequest request) { observer.onNext(EchoResponse.newBuilder().setText(request.getText()).setServerLanguage("Java").build()); }
        @Override public void onError(Throwable error) { observer.onError(error); }
        @Override public void onCompleted() { observer.onCompleted(); }
      };
    }
  }
  private static final class UnprotectedImpl extends UnprotectedServiceGrpc.UnprotectedServiceImplBase {
    @Override public void call(EchoRequest request, StreamObserver<EchoResponse> observer) {
      throw new AssertionError("strict server dispatched an unprotected service");
    }
  }
  public static void main(String[] args) throws Exception {
    io.grpc.Server server = ServerBuilder.forPort(50053)
        .addService(new EchoImpl())
        .addService(new UnprotectedImpl())
        .intercept(Contract.strictServerInterceptor())
        .build().start();
    System.out.println("Java server listening on :50053");
    server.awaitTermination();
  }
}
