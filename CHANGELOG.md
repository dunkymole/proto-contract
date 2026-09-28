# Changelog

All notable changes will be documented here. The project follows Semantic Versioning for releases; protobuf contract versions remain independent.

## Unreleased

- Generate lock-bound Python, Go, Java, and .NET client interceptors; all README client examples and matrix success paths use generated contracts.

- Generate service-specific TypeScript contract interceptors from lock files with `proto-contract generate`; application examples import generated versions.

- TypeScript contract interceptor for grpc-bridge and a real bridge client in the expanded 5-by-4 interoperability matrix.

- Initial contract compiler with deterministic reachable-graph locks.
- Automatic none, minor, and major change classification.
- Python client interceptor and Go, Java, and .NET server adapters.
- Containerized cross-language compatibility demonstration.
- Client and server adapters across Python, Java, .NET, and Go, with a 4-by-4 interoperability matrix.
