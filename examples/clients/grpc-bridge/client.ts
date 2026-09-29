import assert from "node:assert/strict";
import { Code, ConnectError, createClient, createContextValues } from "@connectrpc/connect";
import { openBridgeConnection, waitForReady } from "@dunkymole/grpc-bridge";
import { createBridgeConnection as createRawBridgeConnection, interceptTransport } from "@dunkymole/grpc-bridge/raw";
import { defineContract } from "@dunkymole/grpc-bridge/codegen";
import { EchoService } from "./gen/demo/v1/echo_pb.js";
import { contractClientInterceptor } from "./runtime/proto-contract.js";
import { contract, runtimeGraph } from "./gen/echo_contract.js";

const servers = {
  Go: "go-server:50051",
  Java: "java-server:50053",
  ".NET": "dotnet-server:50054",
  Python: "python-server:50055",
};

for (const [expected, target] of Object.entries(servers)) {
  const connection = await openBridgeConnection({
    url: process.env.BRIDGE_URL ?? "ws://grpc-bridge:8080/tunnel",
    target,
    scheme: "http",
  });
  const generatedClient = connection.client(contract);
  const contractFixture = (api: string, version: string) => connection.client(defineContract({
    service: EchoService, api, version, fingerprint: contract.fingerprint, graph: runtimeGraph,
  }));
  const rawConnection = createRawBridgeConnection({
    url: process.env.BRIDGE_URL ?? "ws://grpc-bridge:8080/tunnel",
    target,
    scheme: "http",
  });
  const rawClient = (api: string, version: string) => createClient(
    EchoService,
    interceptTransport(rawConnection.transport, {
      baseUrl: `http://${target}`,
      interceptors: [contractClientInterceptor(api, version)],
    }),
  );
  const rawUncontracted = createClient(
    EchoService,
    interceptTransport(rawConnection.transport, { baseUrl: `http://${target}` }),
  );
  const call = (stub: typeof generatedClient) => stub.echo(
    { text: "hello", requestId: "grpc-bridge-demo" },
    { timeoutMs: 90_000, contextValues: createContextValues().set(waitForReady, true) },
  );
  const streamCall = async (stub: typeof generatedClient) => {
    const request = { text: "stream", requestId: "grpc-bridge-stream" };
    const options = { timeoutMs: 90_000, contextValues: createContextValues().set(waitForReady, true) };
    const clientStream = await stub.echoClientStream((async function* () { yield request; })(), options);
    assert.equal(clientStream.text, "stream");
    const serverValues: Array<{ text: string; serverLanguage: string }> = [];
    for await (const response of stub.echoServerStream(request, options)) serverValues.push(response);
    assert.equal(serverValues[0]?.text, "stream");
    const duplexValues: Array<{ text: string; serverLanguage: string }> = [];
    for await (const response of stub.echoDuplex((async function* () { yield request; })(), options)) duplexValues.push(response);
    assert.equal(duplexValues[0]?.text, "stream");
  };
  const rejected = (stub: typeof generatedClient) => assert.rejects(call(stub),
    (error: unknown) => error instanceof ConnectError && error.code === Code.FailedPrecondition);
  try {
    // Normal application path: contract identity and version come from the lock.
    const generatedResponse = await call(generatedClient);
    assert.equal(generatedResponse.text, "hello");
    assert.equal(generatedResponse.serverLanguage, expected);
    await streamCall(generatedClient);
    // Explicit contract identities below are compatibility/negative fixtures.
    const older = contractFixture("demo.echo", "1.0.99");
    const newer = contractFixture("demo.echo", "1.2.0");
    const wrongApi = contractFixture("wrong.api", "1.0.0");
    const oldResponse = await call(older);
    assert.equal(oldResponse.serverLanguage, expected);
    // Independent generated clients share one connection, including concurrent calls.
    await Promise.all([
      call(older).then((response) => assert.equal(response.serverLanguage, expected)),
      rejected(newer),
      rejected(wrongApi),
      rejected(contractFixture("demo.echo", "2.0.0")),
      rejected(rawClient("demo.echo", "1.2.0")),
      rejected(rawUncontracted),
    ]);
    await assert.rejects(
      streamCall(rawUncontracted),
      (error: unknown) => error instanceof ConnectError && error.code === Code.FailedPrecondition,
    );
    await assert.rejects(
      streamCall(newer),
      (error: unknown) => error instanceof ConnectError && error.code === Code.FailedPrecondition,
    );
    assert.equal((await call(older)).text, "hello");
    console.log(`PASS TypeScript grpc-bridge client -> ${expected} server`);
  } finally {
    await connection.close();
    await rawConnection.close();
  }
}
console.log("TypeScript grpc-bridge client matrix passed");
