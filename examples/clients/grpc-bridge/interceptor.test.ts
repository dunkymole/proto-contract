import assert from "node:assert/strict";
import test from "node:test";
import { defineContract } from "@dunkymole/grpc-bridge/codegen";
import { EchoService } from "./gen/demo/v1/echo_pb.js";
import { contract, runtimeGraph } from "./gen/echo_contract.js";

test("generated contract carries lock identity and all four actual RPC methods", () => {
  assert.equal(contract.api, "demo.echo");
  assert.equal(contract.version, "1.1.0");
  assert.match(contract.fingerprint, /^[a-f0-9]{64}$/);
  assert.equal(runtimeGraph.service.typeName, EchoService.typeName);
  assert.deepEqual(runtimeGraph.service.methods.map((method) => method.kind).sort(), [
    "bidi_streaming", "client_streaming", "server_streaming", "unary",
  ]);
});

test("generated artifact rejects a stale graph for a same-named service", () => {
  const stale = {
    ...runtimeGraph,
    service: { ...runtimeGraph.service, methods: runtimeGraph.service.methods.slice(1) },
  };
  assert.throws(() => defineContract({
    service: EchoService,
    api: contract.api,
    version: contract.version,
    fingerprint: contract.fingerprint,
    graph: stale,
  }), /runtime graph|descriptor/i);
});

test("generated graph and descriptor agree on the exact service and method set", () => {
  assert.equal(runtimeGraph.service.typeName, EchoService.typeName);
  assert.deepEqual(
    runtimeGraph.service.methods.map((method) => method.name).sort(),
    EchoService.methods.map((method) => method.name).sort(),
  );
});
