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
    // Explicit versions below are compatibility test fixtures, not application configuration.
    for (const version of ["2.0.0", "2.0.1", "2.0.99"]) {
      const response = await call(client("demo.echo", version));
      assert.equal(response.text, "hello");
      assert.equal(response.serverLanguage, expected);
    }
    // Independent generated clients share one connection, including concurrent calls.
    const compatible = client("demo.echo", "2.0.0");
    const newer = client("demo.echo", "2.1.0");
    const wrongApi = client("wrong.api", "2.0.0");
    await Promise.all([
      call(compatible).then((response) => assert.equal(response.serverLanguage, expected)),
      rejected(newer),
      rejected(wrongApi),
      rejected(client("demo.echo", "1.1.0")),
      rejected(createClient(EchoService, connection.transport)),
    ]);
    assert.equal((await call(compatible)).text, "hello");
    console.log(`PASS TypeScript grpc-bridge client -> ${expected} server`);
  } finally {
    await connection.close();
  }
}
console.log("TypeScript grpc-bridge client matrix passed");
