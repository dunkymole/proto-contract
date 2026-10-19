# Proto Contract runtime

Client metadata and strict server enforcement for gRPC. Generate service-specific bindings with the Proto Contract compiler. See https://github.com/dunkymole/proto-contract/blob/main/docs/PACKAGES.md for installation and compatibility policy.

Package version 0.3.0 is independent of your API contract version.

Use generated `Contract.CreateClient(channel)`
for contract-bound clients. ASP.NET Core servers additionally reference
`ProtoContract.AspNetCore`: call `services.AddContractGrpc()` and
`Contract.MapService<Implementation>(app)`.
Mappings register contracts together with endpoints; one global strict guard
denies unlisted services. The core runtime remains usable without ASP.NET Core.
