# Compatibility policy

Proto Contract assigns a version to one protobuf service and everything reachable through its method inputs and outputs.

## Runtime rule

A server accepts a client when both conditions hold:

1. Client and server major versions are equal.
2. The client minor version is less than or equal to the server minor version.

The API identifier must also match. Patch does not affect the comparison. On a registered, supported RPC, missing metadata, a wrong API identifier, or incompatible major/minor versions produce gRPC `FAILED_PRECONDITION` before the application handler runs. See [RPC coverage and registration](RUNTIMES.md) for the scope of each runtime.

The metadata parser is shared across runtimes: the API is 1–64 ASCII characters in `[A-Za-z0-9._:/-]`, the complete value is at most 128 ASCII bytes, and exactly one metadata value must be present. Version components use canonical nonnegative decimal syntax and are each bounded by `2147483647`. Leading zeroes, whitespace, signs, Unicode digits, malformed separators, overflow, and duplicate values fail closed. Generated clients replace any stale value for this key while preserving other metadata.

## Compiler classification

| Change | Minimum bump |
|---|---:|
| No compared contract data change | none |
| Change the selected service name | major |
| Add a method | minor |
| Add a field with a new number | minor |
| Add an enum value with a new number, unless used by a required enum field | minor |
| Add an alias for an existing enum number | major |
| Add an enum value to an enum used by a required field | major |
| Add a required field | major |
| Remove or rename a method | major |
| Change method input, output, or streaming shape | major |
| Remove, rename, renumber, or retype a field | major |
| Remove or change an enum value | major |
| Add a reachable message or enum | minor |
| Remove or rename a reachable message or enum | major |

Field comparison also includes label, referenced type, JSON name, oneof membership, proto3 optional presence, and explicit default values. Default changes require a major bump. Enum values are keyed by name, so aliases sharing a number remain distinct and removing, renaming, or reordering aliases requires a major bump. The default enum name and alias order for each numeric value are captured because they affect implicit values and canonical JSON names. Field and distinct-number enum declaration order is normalized. The digest excludes the lock's version; it identifies the captured structure rather than the application release.

Format 2 supports proto2 and proto3 declarations reachable from the selected service, including field defaults, enum aliases, streaming shape, oneofs, proto3 optional fields, reserved declarations, and deterministic fingerprints of standard descriptor options. Option or reserved declaration changes are conservatively major. Custom or unrecognized options and uninterpreted options are rejected until their extension definitions can be checked. Edition syntax, groups, and extensions that affect reachable declarations are also rejected with an actionable unsupported-feature error. Unrelated declarations in imported files do not affect the snapshot.

The policy is intentionally stricter than protobuf wire compatibility because generated source compatibility matters to application developers.

`check` compares the captured service, method, field, and enum data. `release-check` also validates lock structure and digest, then binds each API/version to the append-only history from the trusted base revision. Neither command is a cryptographic signature. The compiler [workflow](COMPILER.md) explains initialization, intentional updates, and build-time generation.

## Semantic changes

Descriptors cannot reveal every breaking behavior change. The `update --bump` argument lets maintainers raise the computed bump. The tool never permits a lower bump than its structural analysis requires.

## Current limits

- Only protobuf declarations reachable from one named service are considered. File-level and declaration options on files that own reachable declarations are fingerprinted because generated-source semantics may depend on them. Changing an option on an existing declaration is major; options on a newly added declaration follow that declaration's bump, except file-level option additions, which are major.
- Business meaning, validation performed outside protobuf descriptors, and behavior changes inside handlers cannot be inferred; use `update --bump` to raise the version for those changes.
- All four native RPC shapes are enforced by Python, Go, Java, and .NET. The TypeScript grpc-bridge client stamps all four shapes; it relies on the native gRPC server for enforcement. Strict server dispatchers own service-to-contract registration and reject an unregistered service before its handler runs; intentional exemptions must be explicit.
- Lock format 2 is the only supported format for this pre-release. Format 1 and unknown future formats fail with an explicit unsupported-format error. A future format change will be introduced as a deliberate compiler release with a documented parser and transition policy; API semantic versions describe schema compatibility and do not encode the lock serialization format.
