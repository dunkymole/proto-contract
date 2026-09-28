import collections
import grpc

METADATA_KEY = "x-proto-contract"

_ClientCallDetails = collections.namedtuple(
    "_ClientCallDetails", ("method", "timeout", "metadata", "credentials", "wait_for_ready", "compression")
)

class ContractClientInterceptor(grpc.UnaryUnaryClientInterceptor):
    """Adds the application contract version to each unary RPC."""

    def __init__(self, api: str, version: str):
        self._value = f"{api}@{version}"

    def intercept_unary_unary(self, continuation, call_details, request):
        metadata = list(call_details.metadata or ())
        metadata.append((METADATA_KEY, self._value))
        details = _ClientCallDetails(
            call_details.method, call_details.timeout, metadata,
            call_details.credentials, call_details.wait_for_ready, call_details.compression,
        )
        return continuation(details, request)

def intercepted_channel(target: str, api: str, version: str):
    return grpc.intercept_channel(
        grpc.insecure_channel(target), ContractClientInterceptor(api, version)
    )
