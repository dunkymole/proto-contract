import unittest
from types import SimpleNamespace

import grpc
from echo_contract import API, VERSION, SERVICE, ServerInterceptor


class Rejected(Exception):
    pass


class Context:
    def abort(self, code, message):
        if code != grpc.StatusCode.FAILED_PRECONDITION:
            raise AssertionError(code)
        raise Rejected(message)


class GeneratedServerTests(unittest.TestCase):
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
