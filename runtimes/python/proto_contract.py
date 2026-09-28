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

def _parse(value: str):
    parts = value.split(".")
    if len(parts) != 3 or any(not part.isdigit() for part in parts):
        raise ValueError(f"invalid semantic version {value}")
    return tuple(int(part) for part in parts)

def compatibility_error(api: str, offered: str | None, server_version: str):
    prefix = f"{api}@"
    if offered is None:
        return "missing x-proto-contract metadata"
    if not offered.startswith(prefix):
        return f"expected contract {prefix}MAJOR.MINOR.PATCH"
    try:
        client = _parse(offered[len(prefix):])
        server = _parse(server_version)
    except ValueError as error:
        return str(error)
    if client[0] != server[0] or client[1] > server[1]:
        return f"incompatible contract: client {offered[len(prefix):]}, server {server_version}"
    return None

class ContractServerInterceptor(grpc.ServerInterceptor):
    """Rejects incompatible unary RPCs before application code runs."""

    def __init__(self, api: str, version: str):
        self._api = api
        self._version = version

    def intercept_service(self, continuation, call_details):
        handler = continuation(call_details)
        offered = next((value for key, value in call_details.invocation_metadata if key == METADATA_KEY), None)
        error = compatibility_error(self._api, offered, self._version)
        if error is None or handler is None:
            return handler

        def reject(request, context):
            context.abort(grpc.StatusCode.FAILED_PRECONDITION, error)

        return grpc.unary_unary_rpc_method_handler(
            reject,
            request_deserializer=handler.request_deserializer,
            response_serializer=handler.response_serializer,
        )
