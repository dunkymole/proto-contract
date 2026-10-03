# Rolling out contract versions

Proto Contract versions describe a build's protobuf contract; they do not move
an established connection or change a client's generated identity. Roll out
additive changes server-first, keep old clients on their generated contract,
and route breaking majors explicitly to a separate destination.

The executable [rollout scenario](../examples/rollout/) uses genuine generated
schemas and locks:

| Build | Contract | Schema change |
|---|---|---|
| Old | `rollout.echo` 1.0.0 | Unary `Echo` |
| Next | `rollout.echo` 1.1.0 | Adds client-streaming, server-streaming, and bidirectional methods |
| Major | `rollout.echo` 2.0.0 | Changes `EchoRequest.text` from `string` to `bytes` |

The scenario starts all three servers with distinct build IDs. It proves an
old 1.0 client can use the additive 1.1 server, a 1.1 client can use the 1.1
server, and a major 2.0 client reaches only its explicitly selected major
destination. A live 1.1 bidirectional stream stays on its original tunnel while
a new 2.0 connection is opened. The test then sends incompatible clients to
each selected endpoint: each server records exactly one RPC attempt, none of
the application handler counters increase, and no request is replayed to a
different endpoint. Generated client versions remain unchanged. Finally, the
old pinned client and a fresh old-version client both work against the rollback
endpoint.

Run it from the repository root with Docker Compose v2 and PowerShell:

```powershell
./scripts/test-rollout.ps1
```

This uses isolated Compose project names and removes only the resources it
creates. The rollout client image runs the actual descriptor checks for all
three lock/schema pairs and compares v1.1 against v1.0 and v2.0 against v1.1;
the additive methods must classify as minor and the field type change as major.
It also runs the external NodeNext strict-package fixture using the pinned
grpc-bridge package, checking a descriptor graph with imports, maps, defaults,
enums, optional fields, and all four RPC shapes.

The bridge source defaults to reviewed commit
`200f417ce5b070d2c6e5214870a2b0887fa9058f`. To run this proof with another
companion commit, set `GRPC_BRIDGE_REF` before invoking the script; Compose
forwards the same ref to the bridge and client images. The client build derives
the local tarball integrity from the fetched artifact, then checks its
name/version and dependency metadata against the consumer lock before `npm ci`.
Refs whose package dependency graph changes fail until the consumer lock is
reviewed and updated.

```powershell
$env:GRPC_BRIDGE_REF = "<reviewed-commit-sha>"
./scripts/test-rollout.ps1
Remove-Item Env:GRPC_BRIDGE_REF
```

## Deployment guidance

1. Deploy the additive server build everywhere the new client will be routed.
   Keep its old-client acceptance window open; a 1.0 client is accepted by a
   1.1 server because the major matches and the client minor is not newer.
2. Deploy clients generated from 1.1 only after the relevant servers accept
   1.1. A 1.1 client sent to a 1.0 server fails with `FAILED_PRECONDITION`
   before handler execution. The client must not learn a server version or
   retry the call against an unselected destination.
3. For an incompatible major, create an explicit route or destination and
   generate the client from that major's lock. This is explicit routing, not
   automatic load-balancer negotiation. Keep each major's server and client
   identities tied to their own generated lock.
4. For rollback, route old clients back to the old-version server. Keep the
   binary, lock, and destination needed for rollback until the recovery window
   closes. Newer clients will continue to be rejected by that old server;
   rollback does not rewrite their contract or silently downgrade them.

Existing RPCs and streams stay attached to the connection and destination on
which they started. During a server replacement, keep the old build available
until its in-flight streams drain. Set a bounded drain deadline based on the
application's maximum stream lifetime; when that deadline expires, cancel
remaining streams explicitly and let the application decide whether to start
a new operation. Do not transparently replay an interrupted streaming RPC:
the peer may already have processed part or all of it.

The matrix verifies protocol enforcement and routing behavior. It does not
replace application-specific deployment health checks, data migration plans,
or capacity testing. See [compatibility classification](COMPATIBILITY.md) for
what descriptor comparison can and cannot infer.


The [contract-aware routing boundary](ROUTING.md) explains why backend discovery,
version ranking, and channel failover remain responsibilities of the deployment
routing layer rather than Proto Contract itself.
