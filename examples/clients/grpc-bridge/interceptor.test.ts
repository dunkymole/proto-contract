import assert from "node:assert/strict";
import { test } from "node:test";
import { create } from "@bufbuild/protobuf";
import { createClient, createContextValues, type Transport, type UnaryRequest, type StreamRequest } from "@connectrpc/connect";
import { EchoService, EchoRequestSchema } from "./gen/demo/v1/echo_pb.js";
import { contractClientInterceptor, contractClientTransport, metadataKey } from "./runtime/proto-contract.js";

for (const stream of [false, true]) {
  test(`contract metadata preserves ${stream ? "streaming" : "unary"} call state`, async () => {
    let consumed = false;
    const message = create(EchoRequestSchema, { text: "hello" });
    const messages = (async function* () { consumed = true; yield message; })();
    const common = {
      method: EchoService.method.echo,
      service: EchoService,
      url: "http://backend/demo.v1.EchoService/Echo",
      requestMethod: "POST",
      signal: new AbortController().signal,
      contextValues: createContextValues(),
      header: new Headers({ authorization: "Bearer demo", [metadataKey]: "stale@9.0.0" }),
    };
    const request: UnaryRequest | StreamRequest = stream
      ? { ...common, method: { ...common.method, methodKind: "bidi_streaming" }, stream: true, message: messages }
      : { ...common, stream: false, message };
    let calls = 0;
    const invoke = contractClientInterceptor("demo.echo", "1.0.0")(async (forwarded) => {
      calls++;
      assert.equal(forwarded, request);
      assert.equal(forwarded.header.get(metadataKey), "demo.echo@1.0.0");
      assert.equal(forwarded.header.get("authorization"), "Bearer demo");
      assert.equal(forwarded.signal, common.signal);
      assert.equal(forwarded.contextValues, common.contextValues);
      assert.equal(consumed, false);
      return { ...forwarded, trailer: new Headers() };
    });
    const response = await invoke(request);
    assert.equal(calls, 1);
    assert.equal(response.message, request.message);
    assert.equal(consumed, false);
  });
}

test("server errors propagate unchanged", async () => {
  const error = new Error("rejected by server");
  const invoke = contractClientInterceptor("demo.echo", "2.0.0")(async () => { throw error; });
  await assert.rejects(invoke({
    stream: false,
    method: EchoService.method.echo,
    service: EchoService,
    url: "http://backend/demo.v1.EchoService/Echo",
    requestMethod: "POST",
    signal: new AbortController().signal,
    contextValues: createContextValues(),
    header: new Headers(),
    message: create(EchoRequestSchema),
  }), (actual: unknown) => actual === error);
});

test("generated clients share a transport with isolated API names and versions", async () => {
  const seen: Array<string | null> = [];
  const headers = new Headers({ authorization: "Bearer demo", [metadataKey]: "caller@9.0.0" });
  const transport: Transport = {
    async unary(method, _signal, _timeout, header) {
      // Yield so concurrent requests overlap before headers are inspected.
      await Promise.resolve();
      const received = new Headers(header);
      seen.push(received.get(metadataKey));
      assert.equal(received.get("authorization"), "Bearer demo");
      return {
        stream: false, method, service: method.parent,
        message: create(method.output), header: new Headers(), trailer: new Headers(),
      };
    },
    async stream() { throw new Error("unexpected stream"); },
  };
  const first = createClient(EchoService, contractClientTransport(transport, "first.api", "1.0.0"));
  const second = createClient(EchoService, contractClientTransport(transport, "second.api", "2.3.0"));
  await Promise.all([first.echo({}, { headers }), second.echo({}, { headers })]);
  await first.echo({}, { headers });
  await createClient(EchoService, transport).echo({}, { headers });
  assert.deepEqual(seen, ["first.api@1.0.0", "second.api@2.3.0", "first.api@1.0.0", "caller@9.0.0"]);
  assert.equal(headers.get(metadataKey), "caller@9.0.0");
});

test("client transport preserves streaming messages, call options, and errors", async () => {
  const error = new Error("stream rejected");
  const headers = new Headers({ authorization: "Bearer demo" });
  const signal = new AbortController().signal;
  const context = createContextValues();
  let consumed = false;
  const messages = (async function* () { consumed = true; yield create(EchoRequestSchema); })();
  const transport: Transport = {
    async unary() { throw new Error("unexpected unary"); },
    async stream(_method, actualSignal, timeout, header, input, actualContext) {
      assert.equal(new Headers(header).get(metadataKey), "stream.api@3.0.0");
      assert.equal(new Headers(header).get("authorization"), "Bearer demo");
      assert.equal(actualSignal, signal);
      assert.equal(timeout, 5000);
      assert.equal(actualContext, context);
      assert.equal(input, messages);
      assert.equal(consumed, false);
      throw error;
    },
  };
  const wrapped = contractClientTransport(transport, "stream.api", "3.0.0");
  await assert.rejects(wrapped.stream(
    { ...EchoService.method.echo, methodKind: "bidi_streaming" }, signal, 5000, headers, messages, context,
  ), (actual: unknown) => actual === error);
  assert.equal(headers.has(metadataKey), false);
  assert.equal(consumed, false);
});
