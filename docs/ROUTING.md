# Contract-aware routing boundary

Proto Contract determines whether a client contract is acceptable to a server
and enforces that decision on every RPC. It does not discover backends, probe a
fleet, rank compatible versions, or manage channel failover.

Those responsibilities belong to the deployment routing layer: a service
mesh, xDS control plane, load balancer, DNS policy, gateway, or an
application-specific resolver. These systems also have the health, locality,
capacity, authentication, and failure information needed to make a safe
routing decision. A schema version alone is not enough to choose the best
backend.

## Integration model

An external resolver may use a client's generated API identity and version to
select a versioned destination. The selected destination then uses an ordinary
language-native gRPC channel. Proto Contract still stamps every supported RPC,
and the server still rejects a routing mistake before application code runs.

The compatibility predicate is:

```text
client.major == server.major && client.minor <= server.minor
```

This predicate establishes whether a call is allowed; it is not a load-balancer
ranking policy. An operator may prefer an exact version, the closest compatible
minor, the newest healthy deployment, or another policy based on operational
needs. Proto Contract deliberately does not choose among them.

Routing integrations must preserve these safety properties:

- Do not send an application RPC merely to discover compatibility.
- Do not replay a rejected or interrupted application RPC against another
  destination; it may already have produced effects.
- Keep an established RPC or stream on the connection and destination where it
  started.
- Do not rewrite or adopt a different generated client contract at runtime.
- Continue enforcing the contract on every RPC even when the routing layer
  believes the destination is compatible.

The [rollout guide](ROLLOUT.md) demonstrates explicit additive and major-version
routes, pinned streams, rejection without replay, and rollback. A future
first-class discovery protocol should be reconsidered only with concrete user
demand, a cross-language design that composes with native resolvers, and a
clear security and lifecycle model. It is not part of the current roadmap.
