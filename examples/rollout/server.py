from concurrent import futures
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
import threading

import grpc

from demo.v1 import echo_pb2, echo_pb2_grpc
from echo_contract import StrictServerInterceptor


BUILD_ID = os.environ["BUILD_ID"]
_counter_lock = threading.Lock()
_handler_calls = 0
_rpc_attempts = 0


def _record_call():
    global _handler_calls
    with _counter_lock:
        _handler_calls += 1


class AttemptCounterInterceptor(grpc.ServerInterceptor):
    def intercept_service(self, continuation, handler_call_details):
        global _rpc_attempts
        with _counter_lock:
            _rpc_attempts += 1
        return continuation(handler_call_details)


class EchoService(echo_pb2_grpc.EchoServiceServicer):
    def Echo(self, request, context):
        _record_call()
        return echo_pb2.EchoResponse(text=request.text, server_language=BUILD_ID)

    def EchoClientStream(self, request_iterator, context):
        _record_call()
        values = [request.text for request in request_iterator]
        return echo_pb2.EchoResponse(text=values[0] if values else "", server_language=BUILD_ID)

    def EchoServerStream(self, request, context):
        _record_call()
        yield echo_pb2.EchoResponse(text=request.text, server_language=BUILD_ID)

    def EchoDuplex(self, request_iterator, context):
        _record_call()
        for request in request_iterator:
            yield echo_pb2.EchoResponse(text=request.text, server_language=BUILD_ID)


class StateHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path != "/state":
            self.send_error(404)
            return
        with _counter_lock:
            payload = json.dumps({
                "build_id": BUILD_ID,
                "handler_calls": _handler_calls,
                "rpc_attempts": _rpc_attempts,
            }).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)

    def log_message(self, format, *args):
        return


def main():
    server = grpc.server(
        futures.ThreadPoolExecutor(max_workers=8),
        # Place the counter outside strict enforcement so it observes rejected
        # calls too; handler_calls below independently proves they did not run.
        interceptors=(AttemptCounterInterceptor(), StrictServerInterceptor()),
    )
    echo_pb2_grpc.add_EchoServiceServicer_to_server(EchoService(), server)
    if server.add_insecure_port("[::]:50051") == 0:
        raise RuntimeError("could not bind rollout gRPC server")
    admin = ThreadingHTTPServer(("0.0.0.0", 9090), StateHandler)
    threading.Thread(target=admin.serve_forever, daemon=True).start()
    server.start()
    print(f"rollout server {BUILD_ID} listening", flush=True)
    server.wait_for_termination()


if __name__ == "__main__":
    main()
