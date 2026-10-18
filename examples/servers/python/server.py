from concurrent import futures
from demo.v1 import echo_pb2, echo_pb2_grpc
from proto_contract import ServerBuilder
from echo_contract import add_to_server

class EchoService(echo_pb2_grpc.EchoServiceServicer):
    def Echo(self, request, context):
        return echo_pb2.EchoResponse(text=request.text, server_language="Python")

    def EchoClientStream(self, request_iterator, context):
        return echo_pb2.EchoResponse(text=",".join(item.text for item in request_iterator), server_language="Python")

    def EchoServerStream(self, request, context):
        yield echo_pb2.EchoResponse(text=request.text, server_language="Python")

    def EchoDuplex(self, request_iterator, context):
        for item in request_iterator:
            yield echo_pb2.EchoResponse(text=item.text, server_language="Python")

def main():
    builder = ServerBuilder()
    add_to_server(builder, EchoService())
    server = builder.build(futures.ThreadPoolExecutor(max_workers=4))
    server.add_insecure_port("[::]:50055")
    server.start()
    print("Python server listening on :50055", flush=True)
    server.wait_for_termination()

if __name__ == "__main__": main()
