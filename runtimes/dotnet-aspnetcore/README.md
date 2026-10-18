# Proto Contract ASP.NET Core integration

Reference `ProtoContract.AspNetCore` on servers and import `ProtoContract`.

```csharp
builder.Services.AddContractGrpc();
var app = builder.Build();
EchoContract.MapService<EchoServiceImpl>(app);
app.Run();
```

Each mapping registers the endpoint and its generated contract together. The
global strict dispatcher rejects ordinary mapped services without a contract.
Register multiple contracts before starting the host. Duplicate contracts and
service name mismatches fail configuration; registrations freeze at startup.
Pass fully-qualified infrastructure service names to `AddContractGrpc(...)` for
explicit exemptions, then map those services normally.

Client-only applications need just `ProtoContract.Runtime` and the generated
`Contract.CreateClient(channel)` helper. See the repository's runtime
integration guide for the full build workflow. Package version 0.2.1 is
independent of each API's contract version.
