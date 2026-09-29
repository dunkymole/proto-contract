# Changelog

All notable changes will be documented here. The project follows Semantic Versioning for releases; protobuf contract versions remain independent.

## Unreleased

- Initial contract compiler with deterministic reachable-graph locks.
- Automatic none, minor, and major change classification and deliberate lock updates.
- Generated client and server interceptors for Python, Go, Java, and .NET, bound to each build's lock.
- Generated TypeScript client interceptors using grpc-bridge's public per-client transport wrapper.
- Containerized interoperability matrix with five clients and four servers: 20 combinations, including real grpc-bridge traffic.
- Compiler setup/reference, build workflow, generated runtime integration, and coverage documentation.
