# Compatibility policy

Proto Contract assigns a version to one protobuf service and everything reachable through its method inputs and outputs.

## Runtime rule

A server accepts a client when both conditions hold:

1. Client and server major versions are equal.
2. The client minor version is less than or equal to the server minor version.

The API identifier must also match. Patch does not affect the comparison. On a registered, supported RPC, missing metadata, a wrong API identifier, or incompatible major/minor versions produce gRPC `FAILED_PRECONDITION` before the application handler runs. See [RPC coverage and registration](RUNTIMES.md) for the scope of each runtime.

Generated metadata uses numeric `MAJOR.MINOR.PATCH`. Handwritten noncanonical versions and duplicate metadata do not have identical parsing behavior across the current adapters; do not use them as a portable protocol. Register the generated interceptor and avoid adding a second contract header manually.

## Compiler classification

| Change | Minimum bump |
|---|---:|
| No compared contract data change | none |
| Change the selected service name | major |
| Add a method | minor |
| Add a field with a new number | minor |
| Add an enum value | minor |
| Remove or rename a method | major |
| Change method input, output, or streaming shape | major |
| Remove, rename, renumber, or retype a field | major |
| Remove or change an enum value | major |
| Add a reachable message or enum | minor |
| Remove or rename a reachable message or enum | major |

Field comparison also includes label, referenced type, oneof membership, and proto3 optional presence. These changes require a major bump. Declaration order is normalized. The digest excludes the lock's version; it identifies the captured structure rather than the application release.

The policy is intentionally stricter than protobuf wire compatibility because generated source compatibility matters to application developers.

`check` compares the captured service, method, field, and enum data. It is not a byte-for-byte lock comparison or a signature/integrity check. A lock that matches the schema does not prove the recorded version was historically assigned correctly; review and version-control lock changes. The compiler [workflow](COMPILER.md) explains initialization, intentional updates, and build-time generation.

## Semantic changes

Descriptors cannot reveal every breaking behavior change. The `update --bump` argument lets maintainers raise the computed bump. The tool never permits a lower bump than its structural analysis requires.

## Current limits

- Only protobuf declarations reachable from one named service are considered.
- Custom options, validation constraints, HTTP annotations, reserved ranges, default values, and edition features are not yet classified.
- The full runtime matrix covers unary RPCs; some adapters lack streaming hooks. The metadata rule alone does not provide streaming enforcement.
- The lock format has `format: 1` but remains experimental until a stable release.
