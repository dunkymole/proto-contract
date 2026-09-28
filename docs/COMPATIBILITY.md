# Compatibility policy

Proto Contract assigns a version to one protobuf service and everything reachable through its method inputs and outputs.

## Runtime rule

A server accepts a client when both conditions hold:

1. Client and server major versions are equal.
2. The client minor version is less than or equal to the server minor version.

Patch is reserved for changes that do not alter the contract. Missing, malformed, or incompatible metadata produces gRPC `FAILED_PRECONDITION`.

## Compiler classification

| Change | Minimum bump |
|---|---:|
| No canonical descriptor change | none |
| Add a method | minor |
| Add a field with a new number | minor |
| Add an enum value | minor |
| Remove or rename a method | major |
| Change method input, output, or streaming shape | major |
| Remove, rename, renumber, or retype a field | major |
| Remove or change an enum value | major |

The policy is intentionally stricter than protobuf wire compatibility because generated source compatibility matters to application developers.

## Semantic changes

Descriptors cannot reveal every breaking behavior change. The `update --bump` argument lets maintainers raise the computed bump. The tool never permits a lower bump than its structural analysis requires.

## Current limits

- Only protobuf declarations reachable from one named service are considered.
- Custom options, validation constraints, HTTP annotations, reserved ranges, default values, and edition features are not yet classified.
- Runtime helpers currently demonstrate unary RPCs. The wire metadata policy applies equally to streaming RPCs.
- The lock format has `format: 1` but remains experimental until a stable release.
