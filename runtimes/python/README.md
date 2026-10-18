# Proto Contract runtime

Client metadata and strict server enforcement for gRPC. Generate service-specific bindings with the Proto Contract compiler. See https://github.com/dunkymole/proto-contract/blob/main/docs/PACKAGES.md for installation and compatibility policy.

Package version 0.2.1 is independent of your API contract version.

Use generated `contract.client(channel)` to construct a protected client.
Create `proto_contract.ServerBuilder()`, call generated
`contract.add_to_server(builder, implementation)` for
each service, then `builder.build(executor)`. The builder installs one global
strict dispatcher for all RPC shapes. Unlisted raw service registrations fail
closed; explicit service exemptions are accepted by the builder constructor.
Channels and server lifecycle remain owned by the application.
