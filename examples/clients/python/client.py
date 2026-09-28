import sys

import grpc

sys.path.insert(0, "/app/runtime")
from proto_contract import intercepted_channel
from demo.v1 import echo_pb2, echo_pb2_grpc

SERVERS = {
    "Go": "go-server:50051",
    "Java": "java-server:50053",
    ".NET": "dotnet-server:50054",
    "Python": "python-server:50055",
}


def call(target, version):
    channel = intercepted_channel(target, "demo.echo", version)
    stub = echo_pb2_grpc.EchoServiceStub(channel)
    return stub.Echo(echo_pb2.EchoRequest(text="hello", request_id="demo"), timeout=4)


def wait_for(target):
    grpc.channel_ready_future(grpc.insecure_channel(target)).result(timeout=90)


def main():
    for expected, target in SERVERS.items():
        wait_for(target)
        response = call(target, "1.0.0")
        assert response.text == "hello" and response.server_language == expected, response
        for rejected in ("1.2.0", "2.0.0"):
            try:
                call(target, rejected)
                raise AssertionError(f"{expected} accepted incompatible {rejected}")
            except grpc.RpcError as error:
                assert error.code() == grpc.StatusCode.FAILED_PRECONDITION, error
        print(f"PASS Python client -> {expected} server")
    print("Python client matrix passed")


if __name__ == "__main__":
    main()
