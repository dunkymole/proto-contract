import unittest
import json
import pathlib
from concurrent import futures
from types import SimpleNamespace

import grpc
from echo_contract import API, VERSION, SERVICE, ServerInterceptor, StrictServerInterceptor
from proto_contract import compatibility_error, ContractClientInterceptor, StrictServerInterceptor as RuntimeStrictServerInterceptor
from demo.v1 import echo_pb2, echo_pb2_grpc


class Rejected(Exception):
    pass


class Context:
    def abort(self, code, message):
        if code != grpc.StatusCode.FAILED_PRECONDITION:
            raise AssertionError(code)
        raise Rejected(message)


class GeneratedServerTests(unittest.TestCase):
    def test_strict_dispatch_validates_exemptions(self):
        with self.assertRaises(ValueError):
            RuntimeStrictServerInterceptor({}, ["bad/service"])

    def test_shared_vectors(self):
        vectors = json.loads(pathlib.Path("/app/contract-vectors.json").read_text())
        for vector in vectors:
            with self.subTest(vector=vector["name"]):
                try:
                    error = compatibility_error(vector["api"], vector["values"], vector["server"])
                    compatible = error is None
                except ValueError:
                    compatible = False
                self.assertEqual(compatible, vector["compatible"])

    def test_real_streaming_calls_validate_before_handlers(self):
        calls = []
        unprotected_calls = []

        class Service(echo_pb2_grpc.EchoServiceServicer):
            def Echo(self, request, context):
                calls.append("unary")
                return echo_pb2.EchoResponse(text=request.text, server_language="Python")
            def EchoClientStream(self, requests, context):
                calls.append("client-stream")
                return echo_pb2.EchoResponse(text=",".join(r.text for r in requests), server_language="Python")
            def EchoServerStream(self, request, context):
                calls.append("server-stream")
                yield echo_pb2.EchoResponse(text=request.text, server_language="Python")
            def EchoDuplex(self, requests, context):
                calls.append("duplex")
                for request in requests:
                    yield echo_pb2.EchoResponse(text=request.text, server_language="Python")

        server = grpc.server(futures.ThreadPoolExecutor(max_workers=4), interceptors=(StrictServerInterceptor(),))
        echo_pb2_grpc.add_EchoServiceServicer_to_server(Service(), server)

        class Unprotected(echo_pb2_grpc.UnprotectedServiceServicer):
            def Call(self, request, context):
                unprotected_calls.append(request)
                return echo_pb2.EchoResponse(text="must-not-run")

        echo_pb2_grpc.add_UnprotectedServiceServicer_to_server(Unprotected(), server)
        port = server.add_insecure_port("localhost:0")
        server.start()
        try:
            request = echo_pb2.EchoRequest(text="stream")
            def stub_for(interceptor):
                channel = grpc.intercept_channel(grpc.insecure_channel(f"localhost:{port}"), interceptor)
                self.addCleanup(channel.close)
                return echo_pb2_grpc.EchoServiceStub(channel)

            good = stub_for(__import__("echo_contract").ClientInterceptor())
            self.assertEqual(good.Echo(request, timeout=3).text, "stream")
            self.assertEqual(good.EchoClientStream(iter([request]), timeout=3).text, "stream")
            self.assertEqual([r.text for r in good.EchoServerStream(request, timeout=3)], ["stream"])
            self.assertEqual([r.text for r in good.EchoDuplex(iter([request]), timeout=3)], ["stream"])
            self.assertEqual(len(calls), 4)

            unprotected = echo_pb2_grpc.UnprotectedServiceStub(grpc.insecure_channel(f"localhost:{port}"))
            with self.assertRaises(grpc.RpcError) as rejected:
                unprotected.Call(request, timeout=3, metadata=(("x-proto-contract", f"{API}@{VERSION}"),))
            self.assertEqual(rejected.exception.code(), grpc.StatusCode.FAILED_PRECONDITION)
            self.assertEqual(unprotected_calls, [])

            for interceptor in (ContractClientInterceptor(API, "2.2.0", SERVICE), None):
                bad = stub_for(interceptor) if interceptor is not None else echo_pb2_grpc.EchoServiceStub(grpc.insecure_channel(f"localhost:{port}"))
                for shape, invoke in (
                    ("unary", lambda: bad.Echo(request, timeout=3)),
                    ("client-stream", lambda: bad.EchoClientStream(iter([request]), timeout=3).result(timeout=3)),
                    ("server-stream", lambda: list(bad.EchoServerStream(request, timeout=3))),
                    ("duplex", lambda: list(bad.EchoDuplex(iter([request]), timeout=3))),
                ):
                    with self.subTest(interceptor=interceptor is not None, shape=shape), self.assertRaises(grpc.RpcError) as rejected:
                        invoke()
                    self.assertEqual(rejected.exception.code(), grpc.StatusCode.FAILED_PRECONDITION)
                self.assertEqual(len(calls), 4)
        finally:
            server.stop(0).wait()

    def test_lock_compatibility_before_handler(self):
        major, minor, _ = map(int, VERSION.split("."))
        cases = [
            (f"{API}@{VERSION}", True),
            (f"{API}@{major}.{minor}.99", True),
            (f"{API}@{major}.{minor + 1}.0", False),
            (f"{API}@{major + 1}.0.0", False),
            (f"wrong.api@{VERSION}", False),
            (None, False),
        ]
        for offered, allowed in cases:
            with self.subTest(offered=offered):
                called = []
                handler = grpc.unary_unary_rpc_method_handler(
                    lambda request, context: called.append(request)
                )
                details = SimpleNamespace(
                    method=f"/{SERVICE}/Echo",
                    invocation_metadata=[] if offered is None else [("x-proto-contract", offered)],
                )
                wrapped = ServerInterceptor().intercept_service(lambda _: handler, details)
                if allowed:
                    wrapped.unary_unary("payload", Context())
                else:
                    with self.assertRaises(Rejected):
                        wrapped.unary_unary("payload", Context())
                self.assertEqual(called, ["payload"] if allowed else [])

    def test_other_services_keep_their_own_interceptor(self):
        handler = object()
        details = SimpleNamespace(method="/other.Service/Call", invocation_metadata=[])
        self.assertIs(ServerInterceptor().intercept_service(lambda _: handler, details), handler)


if __name__ == "__main__":
    unittest.main()
