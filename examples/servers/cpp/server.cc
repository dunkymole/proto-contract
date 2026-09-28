#include <grpcpp/grpcpp.h>
#include <iostream>
#include <memory>
#include "echo.grpc.pb.h"
#include "contract_verifier.h"

class EchoService final : public demo::v1::EchoService::Service {
  grpc::Status Echo(grpc::ServerContext* context, const demo::v1::EchoRequest* request,
                    demo::v1::EchoResponse* response) override {
    auto contract = proto_contract::Verify(*context, "demo.echo", "1.1.0");
    if (!contract.ok()) return contract;
    response->set_text(request->text());
    response->set_server_language("C++");
    return grpc::Status::OK;
  }
};

int main() {
  EchoService service;
  grpc::ServerBuilder builder;
  builder.AddListeningPort("0.0.0.0:50052", grpc::InsecureServerCredentials());
  builder.RegisterService(&service);
  auto server = builder.BuildAndStart();
  std::cout << "C++ server listening on :50052" << std::endl;
  server->Wait();
}
