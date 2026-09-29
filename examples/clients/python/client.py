import sys

import grpc

sys.path.insert(0, "/app/runtime")
from proto_contract import ContractClientInterceptor
from echo_contract import ClientInterceptor
from demo.v1 import echo_pb2, echo_pb2_grpc

SERVERS = {
    "Go": "go-server:50051",
    "Java": "java-server:50053",
    ".NET": "dotnet-server:50054",
    "Python": "python-server:50055",
}


def call(target, interceptor):
    channel = grpc.intercept_channel(grpc.insecure_channel(target), interceptor)
    stub = echo_pb2_grpc.EchoServiceStub(channel)
    return stub.Echo(echo_pb2.EchoRequest(text="hello", request_id="demo"), timeout=4)


def call_streams(target, interceptor):
    channel = grpc.intercept_channel(grpc.insecure_channel(target), interceptor)
    stub = echo_pb2_grpc.EchoServiceStub(channel)
    request = echo_pb2.EchoRequest(text="stream", request_id="demo")
    response = stub.EchoClientStream(iter([request]), timeout=4)
    assert response.text == "stream", response
    assert [r.text for r in stub.EchoServerStream(request, timeout=4)] == ["stream"]
    assert [r.text for r in stub.EchoDuplex(iter([request]), timeout=4)] == ["stream"]
    channel.close()


def expect_stream_rejected(target, interceptor):
    channel = grpc.insecure_channel(target)
    if interceptor is not None:
        channel = grpc.intercept_channel(channel, interceptor)
    stub = echo_pb2_grpc.EchoServiceStub(channel)
    try:
        list(stub.EchoServerStream(echo_pb2.EchoRequest(text="must-not-run"), timeout=4))
        raise AssertionError("server accepted streaming call without a compatible contract")
    except grpc.RpcError as error:
        assert error.code() == grpc.StatusCode.FAILED_PRECONDITION, error
    finally:
        channel.close()


def wait_for(target):
    grpc.channel_ready_future(grpc.insecure_channel(target)).result(timeout=90)


def main():
    for expected, target in SERVERS.items():
        wait_for(target)
        response = call(target, ClientInterceptor())
        assert response.text == "hello" and response.server_language == expected, response
        call_streams(target, ClientInterceptor())
        expect_stream_rejected(target, ContractClientInterceptor("demo.echo", "1.2.0"))
        expect_stream_rejected(target, None)
        for rejected in ("1.2.0", "2.0.0"):
            try:
                call(target, ContractClientInterceptor("demo.echo", rejected))
                raise AssertionError(f"{expected} accepted incompatible {rejected}")
            except grpc.RpcError as error:
                assert error.code() == grpc.StatusCode.FAILED_PRECONDITION, error
        print(f"PASS Python client -> {expected} server")
    print("Python client matrix passed")


if __name__ == "__main__":
    main()
