import assert from "node:assert/strict";
import { Code, ConnectError, createClient, createContextValues } from "@connectrpc/connect";
import { createBridgeConnection, interceptTransport, waitForReady } from "@dunkymole/grpc-bridge";
import { EchoService } from "./gen/demo/v1/echo_pb.js";
import { contractClientInterceptor } from "./runtime/proto-contract.js";
import { contractInterceptor } from "./gen/echo_contract.js";

const servers = {
  Go: "go-server:50051",
  Java: "java-server:50053",
  ".NET": "dotnet-server:50054",
  Python: "python-server:50055",
};

for (const [expected, target] of Object.entries(servers)) {
  const connection = createBridgeConnection({
    url: process.env.BRIDGE_URL ?? "ws://grpc-bridge:8080/tunnel",
    target,
    scheme: "http",
  });
  const client = (api: string, version: string) => createClient(
    EchoService, interceptTransport(connection.transport, { baseUrl: `http://${target}`, interceptors: [contractClientInterceptor(api, version)] }),
  );
  const call = (stub: ReturnType<typeof client>) => stub.echo(
    { text: "hello", requestId: "grpc-bridge-demo" },
    { timeoutMs: 90_000, contextValues: createContextValues().set(waitForReady, true) },
  );
  const streamCall = async (stub: ReturnType<typeof client>) => {
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
  const rejected = (stub: ReturnType<typeof client>) => assert.rejects(call(stub),
    (error: unknown) => error instanceof ConnectError && error.code === Code.FailedPrecondition);
  try {
    // Normal application path: contract identity and version come from the lock.
    const generatedClient = createClient(EchoService, interceptTransport(connection.transport, {
      baseUrl: `http://${target}`,
      interceptors: [contractInterceptor],
    }));
    const generatedResponse = await call(generatedClient);
    assert.equal(generatedResponse.text, "hello");
    assert.equal(generatedResponse.serverLanguage, expected);
    await streamCall(generatedClient);
    // Explicit versions below are compatibility test fixtures, not application configuration.
    for (const version of ["1.0.0", "1.0.1", "1.0.99"]) {
      const response = await call(client("demo.echo", version));
      assert.equal(response.text, "hello");
      assert.equal(response.serverLanguage, expected);
    }
    // Independent generated clients share one connection, including concurrent calls.
    const compatible = client("demo.echo", "1.0.0");
    const newer = client("demo.echo", "1.2.0");
    const wrongApi = client("wrong.api", "1.0.0");
    await Promise.all([
      call(compatible).then((response) => assert.equal(response.serverLanguage, expected)),
      rejected(newer),
      rejected(wrongApi),
      rejected(client("demo.echo", "1.2.0")),
      rejected(client("demo.echo", "2.0.0")),
      rejected(createClient(EchoService, connection.transport)),
    ]);
    await assert.rejects(
      streamCall(createClient(EchoService, connection.transport)),
      (error: unknown) => error instanceof ConnectError && error.code === Code.FailedPrecondition,
    );
    await assert.rejects(
      streamCall(client("demo.echo", "1.2.0")),
      (error: unknown) => error instanceof ConnectError && error.code === Code.FailedPrecondition,
    );
    assert.equal((await call(compatible)).text, "hello");
    console.log(`PASS TypeScript grpc-bridge client -> ${expected} server`);
  } finally {
    await connection.close();
  }
}
console.log("TypeScript grpc-bridge client matrix passed");
