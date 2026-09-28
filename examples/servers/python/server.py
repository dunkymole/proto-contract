from concurrent import futures
import grpc

from demo.v1 import echo_pb2, echo_pb2_grpc
from proto_contract import ContractServerInterceptor

class EchoService(echo_pb2_grpc.EchoServiceServicer):
    def Echo(self, request, context):
        return echo_pb2.EchoResponse(text=request.text, server_language="Python")

def main():
    server = grpc.server(
        futures.ThreadPoolExecutor(max_workers=4),
        interceptors=(ContractServerInterceptor("demo.echo", "1.1.0"),),
    )
    echo_pb2_grpc.add_EchoServiceServicer_to_server(EchoService(), server)
    server.add_insecure_port("[::]:50055")
    server.start()
    print("Python server listening on :50055", flush=True)
    server.wait_for_termination()

if __name__ == "__main__": main()
