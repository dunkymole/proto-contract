import assert from "node:assert/strict";
import { Code, ConnectError, createContextValues } from "@connectrpc/connect";
import { openBridgeConnection, waitForReady } from "@dunkymole/grpc-bridge";
import { contract as v1_0 } from "./gen/v1_0/echo_contract.js";
import { contract as v1_1 } from "./gen/v1_1/echo_contract.js";
import { contract as v2_0 } from "./gen/v2_0/echo_contract.js";
import type { EchoRequest as V1Request } from "./gen/v1_1/demo/v1/echo_pb.js";

const bridge = process.env.BRIDGE_URL ?? "ws://rollout-bridge:8080/tunnel";
const destinations = {
  old: "rollout-old:50051",
  next: "rollout-new:50051",
  major: "rollout-major:50051",
} as const;
const timeoutMs = 5_000;
const callOptions = () => ({ timeoutMs, contextValues: createContextValues().set(waitForReady, true) });

type ServerState = { build_id: string; handler_calls: number; rpc_attempts: number };

async function state(destination: string): Promise<ServerState> {
  const response = await fetch(`http://${destination.replace(":50051", ":9090")}/state`, {
    signal: AbortSignal.timeout(2_000),
  });
  assert.equal(response.status, 200);
  return response.json() as Promise<ServerState>;
}

async function ready(): Promise<void> {
  const started = Date.now();
  while (Date.now() - started < 30_000) {
    try {
      const states = await Promise.all(Object.values(destinations).map(state));
      if (states.every((item) => item.build_id !== "")) return;
    } catch {
      await new Promise((resolve) => setTimeout(resolve, 150));
    }
  }
  throw new Error("rollout servers did not become ready");
}

function queue<T>() {
  const items: T[] = [];
  const readers: Array<(value: IteratorResult<T>) => void> = [];
  let closed = false;
  return {
    push(value: T) {
      const reader = readers.shift();
      if (reader) reader({ done: false, value });
      else items.push(value);
    },
    close() {
      closed = true;
      for (const reader of readers.splice(0)) reader({ done: true, value: undefined });
    },
    next(): Promise<IteratorResult<T>> {
      const value = items.shift();
      if (value !== undefined) return Promise.resolve({ done: false, value });
      if (closed) return Promise.resolve({ done: true, value: undefined });
      return new Promise((resolve) => readers.push(resolve));
    },
    [Symbol.asyncIterator]() { return this; },
  };
}

async function expectRejected(call: Promise<unknown>, label: string): Promise<void> {
  const started = Date.now();
  await assert.rejects(call, (error: unknown) =>
    error instanceof ConnectError && error.code === Code.FailedPrecondition,
  );
  assert.ok(Date.now() - started < timeoutMs + 1_000, `${label} waited instead of failing promptly`);
}

await ready();
const openedConnections: Array<Awaited<ReturnType<typeof openBridgeConnection>>> = [];
try {
assert.equal(v1_0.version, "1.0.0");
assert.equal(v1_1.version, "1.1.0");
assert.equal(v2_0.version, "2.0.0");
assert.ok([v1_0, v1_1, v2_0].every((item) => item.api === "rollout.echo"));

// This connection remains tied to the old endpoint after later dials select
// different destinations; the bridge does not move an established tunnel.
const pinnedOld = await openBridgeConnection({ url: bridge, target: destinations.old });
openedConnections.push(pinnedOld);
const oldClient = pinnedOld.client(v1_0);
const oldInitial = await oldClient.echo({ text: "old", requestId: "before-rollout" }, callOptions());
assert.equal(oldInitial.serverLanguage, "v1.0.0");

const freshNext = await openBridgeConnection({ url: bridge, target: destinations.next });
openedConnections.push(freshNext);
const nextClient = freshNext.client(v1_1);
const oldClientOnNext = freshNext.client(v1_0);
const oldThroughNew = await oldClientOnNext.echo({ text: "old-client", requestId: "additive-rollout" }, callOptions());
assert.equal(oldThroughNew.serverLanguage, "v1.1.0");
const nextResponse = await nextClient.echo({ text: "new-client", requestId: "additive-rollout" }, callOptions());
assert.equal(nextResponse.serverLanguage, "v1.1.0");

// Keep a bidi RPC open while dialing an explicitly separate major destination.
// Subsequent messages on that stream still reach the original v1.1 process.
const requests = queue<V1Request>();
const stream = nextClient.echoDuplex(requests, callOptions());
const streamIterator = stream[Symbol.asyncIterator]();
requests.push({ $typeName: "demo.v1.EchoRequest", text: "stream-before-new-dial", requestId: "pinned-stream" });
const firstStreamResponse = await streamIterator.next();
if (firstStreamResponse.done) throw new Error("v1.1 bidi stream ended before the first response");
assert.equal(firstStreamResponse.value.serverLanguage, "v1.1.0");

const majorConnection = await openBridgeConnection({ url: bridge, target: destinations.major });
openedConnections.push(majorConnection);
const majorClient = majorConnection.client(v2_0);
const majorResponse = await majorClient.echo({ text: new TextEncoder().encode("major"), requestId: "separate-major" }, callOptions());
assert.equal(new TextDecoder().decode(majorResponse.text), "major");
assert.equal(majorResponse.serverLanguage, "v2.0.0");
requests.push({ $typeName: "demo.v1.EchoRequest", text: "stream-after-new-dial", requestId: "pinned-stream" });
const secondStreamResponse = await streamIterator.next();
if (secondStreamResponse.done) throw new Error("v1.1 bidi stream ended after a fresh major-version dial");
assert.equal(secondStreamResponse.value.serverLanguage, "v1.1.0");
requests.close();
assert.equal((await streamIterator.next()).done, true);

// FAILED_PRECONDITION must not invoke a handler or fall through to another
// destination. It also must not mutate any generated client version.
const beforeRejects = await Promise.all(Object.values(destinations).map(state));
await expectRejected(
  pinnedOld.client(v1_1).echo({ text: "new-to-old", requestId: "rollback" }, callOptions()).then(() => {
    throw new Error("v1.1 client unexpectedly succeeded against v1.0 rollback endpoint");
  }),
  "v1.1 client to v1.0 endpoint",
);
await expectRejected(
  freshNext.client(v2_0).echo({ text: new TextEncoder().encode("wrong-major"), requestId: "wrong-major" }, callOptions()),
  "v2.0 client to v1 endpoint",
);
await expectRejected(
  majorConnection.client(v1_0).echo({ text: "wrong-major", requestId: "wrong-major" }, callOptions()),
  "v1.0 client to v2 endpoint",
);
const afterRejects = await Promise.all(Object.values(destinations).map(state));
assert.deepEqual(
  afterRejects.map((item) => item.handler_calls),
  beforeRejects.map((item) => item.handler_calls),
  "incompatible RPC ran a handler",
);
assert.deepEqual(
  afterRejects.map((item) => item.rpc_attempts),
  beforeRejects.map((item) => item.rpc_attempts + 1),
  "each rejected RPC should be attempted once at its selected endpoint without replay or fallback",
);
assert.equal(v1_1.version, "1.1.0", "server state must not be adopted into the generated client");

// Rollback remains usable by clients at the restored version, while clients
// containing the additive API fail clearly until the v1.1 server returns.
const oldAfterRollback = await oldClient.echo({ text: "rollback-old-client", requestId: "rollback" }, callOptions());
assert.equal(oldAfterRollback.serverLanguage, "v1.0.0");
const rollbackDial = await openBridgeConnection({ url: bridge, target: destinations.old });
openedConnections.push(rollbackDial);
const redialedOld = await rollbackDial.client(v1_0).echo({ text: "fresh-rollback-dial", requestId: "rollback" }, callOptions());
assert.equal(redialedOld.serverLanguage, "v1.0.0");
const pinnedAfterNewDial = await oldClient.echo({ text: "still-pinned", requestId: "pinned" }, callOptions());
assert.equal(pinnedAfterNewDial.serverLanguage, "v1.0.0");

console.log("PASS genuine v1.0 -> v1.1 -> v2.0 routing, pinned tunnel, stream, and rollback scenarios");
} finally {
  await Promise.allSettled(openedConnections.map((connection) => connection.close()));
}
