"""Shared gRPC contract metadata rules and interceptors.

Wire value: ``API@MAJOR.MINOR.PATCH``. API is 1-64 ASCII letters, digits,
dot, underscore, colon, slash, or hyphen. Version components
are canonical unsigned decimal integers in [0, 2147483647]. The complete
value is at most 128 ASCII bytes. A call must carry exactly one value.
"""
import collections
import re
import grpc

METADATA_KEY = "x-proto-contract"
MAX_METADATA_LENGTH = 128
_API = re.compile(r"[A-Za-z0-9._:/-]{1,64}\Z", re.ASCII)
_SERVICE = re.compile(r"[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*\Z", re.ASCII)
_VERSION = re.compile(r"(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\Z", re.ASCII)
_MAX_COMPONENT = 2147483647
_ClientCallDetails = collections.namedtuple(
    "_ClientCallDetails", ("method", "timeout", "metadata", "credentials", "wait_for_ready", "compression")
)


def validate_configuration(api: str, version: str) -> str:
    if not isinstance(api, str) or not _API.fullmatch(api):
        raise ValueError("invalid contract API identifier")
    _parse(version)
    value = f"{api}@{version}"
    if len(value.encode("ascii")) > MAX_METADATA_LENGTH:
        raise ValueError("contract metadata exceeds 128 ASCII bytes")
    return value


def _parse(version: str):
    if not isinstance(version, str):
        raise ValueError("invalid semantic version")
    match = _VERSION.fullmatch(version)
    if not match:
        raise ValueError(f"invalid semantic version {version}")
    parts = tuple(int(p) for p in match.groups())
    if any(part > _MAX_COMPONENT for part in parts):
        raise ValueError(f"invalid semantic version {version}")
    return parts


def compatibility_error(api: str, offered_values, server_version: str):
    """Return a shared diagnostic or None. offered_values is a value sequence."""
    try:
        server = _parse(server_version)
        validate_configuration(api, server_version)
    except (ValueError, UnicodeError):
        raise
    values = list(offered_values or ())
    if not values:
        return "missing x-proto-contract metadata"
    if len(values) != 1:
        return "duplicate x-proto-contract metadata"
    offered = values[0]
    if not isinstance(offered, str) or not offered.isascii() or len(offered.encode("ascii")) > MAX_METADATA_LENGTH:
        return "invalid x-proto-contract metadata"
    if "@" not in offered:
        return "expected contract API@MAJOR.MINOR.PATCH"
    offered_api, version = offered.split("@", 1)
    if not _API.fullmatch(offered_api) or offered_api != api:
        return "expected contract API@MAJOR.MINOR.PATCH"
    try:
        client = _parse(version)
    except ValueError:
        return "invalid semantic version " + version
    if client[0] != server[0] or client[1] > server[1]:
        return f"incompatible contract: client {version}, server {server_version}"
    return None


def _method_service(method):
    if not isinstance(method, str) or not method.startswith("/") or "/" not in method[1:]:
        return None
    return method[1:].rsplit("/", 1)[0]


def _replace_metadata(call_details, value):
    metadata = [(key, item) for key, item in (call_details.metadata or ()) if key.lower() != METADATA_KEY]
    metadata.append((METADATA_KEY, value))
    return _ClientCallDetails(call_details.method, call_details.timeout, metadata,
                              call_details.credentials, call_details.wait_for_ready, call_details.compression)


class _ClientMixin:
    def __init__(self, api: str, version: str, service: str | None = None):
        self._value = validate_configuration(api, version)
        if service is not None and not _SERVICE.fullmatch(service):
            raise ValueError(f"invalid protobuf service name {service}")
        self._service = service

    def _details(self, call_details):
        if self._service is not None and _method_service(call_details.method) != self._service:
            return call_details
        return _replace_metadata(call_details, self._value)


class ContractClientInterceptor(_ClientMixin,
        grpc.UnaryUnaryClientInterceptor, grpc.UnaryStreamClientInterceptor,
        grpc.StreamUnaryClientInterceptor, grpc.StreamStreamClientInterceptor):
    """Contract metadata for all call shapes; optionally scoped to one service."""
    def intercept_unary_unary(self, continuation, call_details, request):
        return continuation(self._details(call_details), request)
    def intercept_unary_stream(self, continuation, call_details, request):
        return continuation(self._details(call_details), request)
    def intercept_stream_unary(self, continuation, call_details, request_iterator):
        return continuation(self._details(call_details), request_iterator)
    def intercept_stream_stream(self, continuation, call_details, request_iterator):
        return continuation(self._details(call_details), request_iterator)


def intercepted_channel(target: str, api: str, version: str, service: str | None = None):
    return grpc.intercept_channel(grpc.insecure_channel(target), ContractClientInterceptor(api, version, service))


def _rejecting_handler(handler, error):
    def reject(request_or_iterator, context):
        context.abort(grpc.StatusCode.FAILED_PRECONDITION, error)
    options = {"request_deserializer": handler.request_deserializer,
               "response_serializer": handler.response_serializer}
    if handler.request_streaming and handler.response_streaming:
        return grpc.stream_stream_rpc_method_handler(reject, **options)
    if handler.request_streaming:
        return grpc.stream_unary_rpc_method_handler(reject, **options)
    if handler.response_streaming:
        return grpc.unary_stream_rpc_method_handler(reject, **options)
    return grpc.unary_unary_rpc_method_handler(reject, **options)


class ContractServerInterceptor(grpc.ServerInterceptor):
    """Reject incompatible calls before application code, preserving handler shape."""
    def __init__(self, api: str, version: str, service: str | None = None):
        self._api = api
        self._version = version
        validate_configuration(api, version)
        if service is not None and not _SERVICE.fullmatch(service):
            raise ValueError(f"invalid protobuf service name {service}")
        self._service = service

    def intercept_service(self, continuation, call_details):
        handler = continuation(call_details)
        if handler is None or (self._service is not None and _method_service(call_details.method) != self._service):
            return handler
        values = [value for key, value in (call_details.invocation_metadata or ())
                  if key.lower() == METADATA_KEY]
        error = compatibility_error(self._api, values, self._version)
        return handler if error is None else _rejecting_handler(handler, error)


def validate_service_coverage(protected_services, registered_services, exemptions=()):
    """Fail at startup if any registered application service lacks protection.

    protected_services and registered_services are fully-qualified protobuf
    service names. Every registered service must be protected or explicitly
    exempted (for example, grpc.health.v1.Health).
    """
    protected, registered, exempt = set(protected_services), set(registered_services), set(exemptions)
    missing = sorted(registered - protected - exempt)
    stale = sorted(protected - registered)
    if missing or stale:
        raise ValueError(f"contract service coverage incomplete: unprotected={missing}, unregistered={stale}")


class StrictServerInterceptor(grpc.ServerInterceptor):
    """Fail-closed service dispatch: every service needs a contract or exemption."""
    def __init__(self, contracts, exemptions=()):
        self._contracts = dict(contracts)
        self._exemptions = set(exemptions)
        for service, interceptor in self._contracts.items():
            if not re.fullmatch(r"[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*", service):
                raise ValueError(f"invalid protobuf service name {service}")
            if not isinstance(interceptor, ContractServerInterceptor) or interceptor._service != service:
                raise ValueError(f"contract interceptor is not bound to service {service}")
        if any(not isinstance(service, str) or not _SERVICE.fullmatch(service) for service in self._exemptions):
            raise ValueError("invalid protobuf service name in exemptions")
        if self._exemptions & self._contracts.keys():
            raise ValueError("service cannot be both protected and exempt")

    def intercept_service(self, continuation, call_details):
        service = _method_service(call_details.method)
        contract = self._contracts.get(service)
        if contract is not None:
            return contract.intercept_service(continuation, call_details)
        handler = continuation(call_details)
        if handler is None or service in self._exemptions:
            return handler
        return _rejecting_handler(handler, "unprotected service registration")
