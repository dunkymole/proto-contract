# Proto Contract runtime

Client metadata and strict server enforcement for gRPC. Generate service-specific bindings with the Proto Contract compiler. See https://github.com/dunkymole/proto-contract/blob/main/docs/PACKAGES.md for installation and compatibility policy.

Package version 0.4.0 is independent of your API contract version.

Use generated `Contract.blockingClient(channel)` (or
`client(channel)`/`futureClient(channel)`) to construct a protected client. Wrap your native
`ServerBuilder` with `ContractServerBuilder`, call generated
`Contract.addService(builder, implementation)` for each contract, then `build()`.
The builder installs one global strict dispatcher; ordinary services added to the
native builder are denied unless explicitly exempted by fully-qualified name.
Channels and server lifecycle remain owned by the application.
