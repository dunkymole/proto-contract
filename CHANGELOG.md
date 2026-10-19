# Changelog

All notable changes will be documented here. The project follows Semantic Versioning for releases; protobuf contract versions remain independent.

## 0.3.0 (unreleased)

- Upgrade the Go build baseline to 1.27.2, require Go 1.26.9+, update gRPC-Go and x/net to patched releases, and gate CI/release preparation on reachable vulnerability scans.
- Contract-bound native client factories and typed server registration with global fail-closed enforcement, descriptor-based binding inference, independent overrides, naming conformance tests, and ASP.NET Core integration.

- Manual release preparation workflow with downloadable package assets, SHA-256 checksums and optional unpublished GitHub draft Releases.
- Unpublished Python, Java, .NET, and Go runtime artifacts with license metadata, checksums, package-based interoperability tests, and a clean Python starter.
- Initial contract compiler with deterministic reachable-graph locks.
- Automatic none, minor, and major change classification and deliberate lock updates.
- Generated client and server interceptors for Python, Go, Java, and .NET, bound to each build's lock.
- Generated TypeScript client interceptors using grpc-bridge's public per-client transport wrapper.
- Containerized interoperability matrix with five clients and four servers: 20 combinations, including real grpc-bridge traffic.
- Compiler setup/reference, build workflow, generated runtime integration, and coverage documentation.
