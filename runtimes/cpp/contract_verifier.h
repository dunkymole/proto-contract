#pragma once

#include <grpcpp/server_context.h>
#include <grpcpp/support/status.h>
#include <sstream>
#include <string>

namespace proto_contract {

struct Version { int major, minor, patch; };

inline bool ParseVersion(const std::string& value, Version* out) {
  char a, b; std::istringstream in(value);
  if (!(in >> out->major >> a >> out->minor >> b >> out->patch) || a != '.' || b != '.' || !in.eof()) return false;
  return out->major >= 0 && out->minor >= 0 && out->patch >= 0;
}

inline grpc::Status Verify(const grpc::ServerContext& context, const std::string& api,
                           const std::string& server_version) {
  const auto& metadata = context.client_metadata();
  auto found = metadata.find("x-proto-contract");
  if (found == metadata.end()) return {grpc::StatusCode::FAILED_PRECONDITION, "missing x-proto-contract metadata"};
  std::string offered(found->second.data(), found->second.length());
  const std::string prefix = api + "@";
  if (offered.rfind(prefix, 0) != 0)
    return {grpc::StatusCode::FAILED_PRECONDITION, "expected contract " + prefix + "MAJOR.MINOR.PATCH"};
  Version client, server;
  if (!ParseVersion(offered.substr(prefix.size()), &client) || !ParseVersion(server_version, &server))
    return {grpc::StatusCode::FAILED_PRECONDITION, "invalid semantic version"};
  if (client.major != server.major || client.minor > server.minor)
    return {grpc::StatusCode::FAILED_PRECONDITION, "incompatible contract: client " + offered.substr(prefix.size()) + ", server " + server_version};
  return grpc::Status::OK;
}
}
