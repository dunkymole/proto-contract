package io.github.dunkymole.protocontract.demo;

import io.grpc.ServerBuilder;
import io.grpc.ServerInterceptors;
import io.grpc.stub.StreamObserver;
import io.github.dunkymole.protocontract.ContractServerInterceptor;
import io.github.dunkymole.protocontract.demo.v1.*;

public final class Server {
  private static final class EchoImpl extends EchoServiceGrpc.EchoServiceImplBase {
    @Override public void echo(EchoRequest request, StreamObserver<EchoResponse> observer) {
      observer.onNext(EchoResponse.newBuilder().setText(request.getText()).setServerLanguage("Java").build());
      observer.onCompleted();
    }
  }
  public static void main(String[] args) throws Exception {
    io.grpc.Server server = ServerBuilder.forPort(50053)
        .addService(ServerInterceptors.intercept(new EchoImpl(), new ContractServerInterceptor("demo.echo", "1.1.0")))
        .build().start();
    System.out.println("Java server listening on :50053");
    server.awaitTermination();
  }
}
