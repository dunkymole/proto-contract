# Changelog

All notable changes will be documented here. The project follows Semantic Versioning for releases; protobuf contract versions remain independent.

## 0.5.0 (2026-10-20)

- Support Edition 2023's six core protobuf features with pinned defaults, lexical inheritance, canonical contract fingerprints, and matching CLI/protoc-plugin generation. Equivalent resolved settings retain the same digest; feature changes require a major API contract bump.
- Verify Edition 2023 runtime graphs against genuine Protobuf-ES bindings, including stale UTF-8 policy rejection. Later editions, language-specific features, invalid combinations, and edition-based custom-option definitions continue to fail closed.
- Reject ambiguous custom-option extension numbers in imported and nested definitions, both when building contracts and validating supplied locks.
- Align compiler/runtime packages, consumer examples, and installation references at this release. Existing proto2/proto3 locks retain their semantics; Edition 2023 locks require this compiler or newer. Native protobuf binding generators and runtimes must also support the edition.
- Add checked version updates, read-only candidate preparation, explicit promotion of tested artifacts, resumable publication and public-download/Go verification, with a next-version dry run.

## 0.4.0 (2026-10-20)

- Resolve imported custom protobuf options, retain checked definitions and values in contract locks, and require a major contract bump for annotation changes. Validate source-retained plugin options without adding edition support.

## 0.3.0 (2026-10-19)

- Upgrade the Go build baseline to 1.27.2, require Go 1.26.9+, update gRPC-Go and x/net to patched releases, and gate CI/release preparation on reachable vulnerability scans.
- Contract-bound native client factories and typed server registration with global fail-closed enforcement, descriptor-based binding inference, independent overrides, naming conformance tests, and ASP.NET Core integration.

- Manual release preparation workflow with downloadable package assets, SHA-256 checksums and optional unpublished GitHub draft Releases.
- Published Python, Java, .NET, and Go runtime artifacts on [GitHub Releases](https://github.com/dunkymole/proto-contract/releases/tag/v0.3.0), with license metadata, checksums, package-based interoperability tests, and a clean Python starter. Go is also available through the public module tag.
- Initial contract compiler with deterministic reachable-graph locks.
- Automatic none, minor, and major change classification and deliberate lock updates.
- Generated client and server interceptors for Python, Go, Java, and .NET, bound to each build's lock.
- Generated TypeScript client interceptors using grpc-bridge's public per-client transport wrapper.
- Containerized interoperability matrix with five clients and four servers: 20 combinations, including real grpc-bridge traffic.
- Compiler setup/reference, build workflow, generated runtime integration, and coverage documentation.
