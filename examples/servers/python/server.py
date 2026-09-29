from concurrent import futures
import grpc

from demo.v1 import echo_pb2, echo_pb2_grpc
from echo_contract import StrictServerInterceptor

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
    server = grpc.server(
        futures.ThreadPoolExecutor(max_workers=4),
        interceptors=(StrictServerInterceptor(),),
    )
    echo_pb2_grpc.add_EchoServiceServicer_to_server(EchoService(), server)
    server.add_insecure_port("[::]:50055")
    server.start()
    print("Python server listening on :50055", flush=True)
    server.wait_for_termination()

if __name__ == "__main__": main()
