"""A clean consumer: imports an installed runtime and generated application code."""
from concurrent import futures
import grpc
import proto_contract
from echo_contract import ClientInterceptor, StrictServerInterceptor
from demo.v1 import echo_pb2, echo_pb2_grpc


class Echo(echo_pb2_grpc.EchoServiceServicer):
    calls = 0

    def Echo(self, request, context):
        self.calls += 1
        return echo_pb2.EchoResponse(text=request.text)


def main():
    assert 'site-packages' in proto_contract.__file__, proto_contract.__file__
    handler = Echo()
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=2),
                         interceptors=(StrictServerInterceptor(),))
    echo_pb2_grpc.add_EchoServiceServicer_to_server(handler, server)
    port = server.add_insecure_port('127.0.0.1:0')
    server.start()
    try:
        with grpc.insecure_channel(f'127.0.0.1:{port}') as channel:
            client = echo_pb2_grpc.EchoServiceStub(grpc.intercept_channel(channel, ClientInterceptor()))
            assert client.Echo(echo_pb2.EchoRequest(text='hello'), timeout=5).text == 'hello'
            print('PASS: generated client accepted')
            bare = echo_pb2_grpc.EchoServiceStub(channel)
            for metadata in ((), (('x-proto-contract', 'demo.echo@2.0.0'),)):
                try:
                    bare.Echo(echo_pb2.EchoRequest(text='must not run'), metadata=metadata, timeout=5)
                except grpc.RpcError as error:
                    assert error.code() == grpc.StatusCode.FAILED_PRECONDITION, error
                else:
                    raise AssertionError('invalid contract accepted')
            assert handler.calls == 1, handler.calls
            print('PASS: missing and incompatible contracts rejected before handler entry')
    finally:
        server.stop(0).wait()


if __name__ == '__main__':
    main()
