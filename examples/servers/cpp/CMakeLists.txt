cmake_minimum_required(VERSION 3.16)
project(proto_contract_demo LANGUAGES CXX)
set(CMAKE_CXX_STANDARD 17)
find_package(Protobuf REQUIRED)
find_package(gRPC CONFIG REQUIRED)
set(PROTO_FILE ${CMAKE_CURRENT_SOURCE_DIR}/proto/demo/v1/echo.proto)
get_filename_component(PROTO_DIR ${CMAKE_CURRENT_SOURCE_DIR}/proto ABSOLUTE)
add_custom_command(OUTPUT ${CMAKE_CURRENT_BINARY_DIR}/echo.pb.cc ${CMAKE_CURRENT_BINARY_DIR}/echo.pb.h ${CMAKE_CURRENT_BINARY_DIR}/echo.grpc.pb.cc ${CMAKE_CURRENT_BINARY_DIR}/echo.grpc.pb.h
  COMMAND protobuf::protoc --proto_path=${PROTO_DIR} --cpp_out=${CMAKE_CURRENT_BINARY_DIR} --grpc_out=${CMAKE_CURRENT_BINARY_DIR} --plugin=protoc-gen-grpc=$<TARGET_FILE:gRPC::grpc_cpp_plugin> ${PROTO_FILE}
  COMMAND ${CMAKE_COMMAND} -E copy ${CMAKE_CURRENT_BINARY_DIR}/demo/v1/echo.pb.cc ${CMAKE_CURRENT_BINARY_DIR}/echo.pb.cc
  COMMAND ${CMAKE_COMMAND} -E copy ${CMAKE_CURRENT_BINARY_DIR}/demo/v1/echo.pb.h ${CMAKE_CURRENT_BINARY_DIR}/echo.pb.h
  COMMAND ${CMAKE_COMMAND} -E copy ${CMAKE_CURRENT_BINARY_DIR}/demo/v1/echo.grpc.pb.cc ${CMAKE_CURRENT_BINARY_DIR}/echo.grpc.pb.cc
  COMMAND ${CMAKE_COMMAND} -E copy ${CMAKE_CURRENT_BINARY_DIR}/demo/v1/echo.grpc.pb.h ${CMAKE_CURRENT_BINARY_DIR}/echo.grpc.pb.h
  DEPENDS ${PROTO_FILE})
add_executable(server server.cc ${CMAKE_CURRENT_BINARY_DIR}/echo.pb.cc ${CMAKE_CURRENT_BINARY_DIR}/echo.grpc.pb.cc)
target_include_directories(server PRIVATE ${CMAKE_CURRENT_BINARY_DIR} ${CMAKE_CURRENT_SOURCE_DIR}/runtime)
target_link_libraries(server PRIVATE gRPC::grpc++ protobuf::libprotobuf)
