import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createFileRegistry, fromBinary } from "@bufbuild/protobuf";
import { FileDescriptorSetSchema } from "@bufbuild/protobuf/wkt";
import { defineContract } from "@dunkymole/grpc-bridge/codegen";
import { createBridgeConnection } from "@dunkymole/grpc-bridge";
import { contract, runtimeGraph } from "./manager_contract.js";
import { ContractService } from "./manager_pb.js";

assert.equal(contract.api, "review.binding");
assert.equal(contract.version, "1.0.0");
assert.equal(runtimeGraph.service.methods.length, 4);
assert.ok(Object.isFrozen(ContractService));
const connection = createBridgeConnection({ url: "ws://127.0.0.1:1/tunnel" });
try {
  assert.equal(typeof connection.client(contract).get_URL, "function");
} finally {
  await connection.close();
}
const descriptorSet = fromBinary(FileDescriptorSetSchema, readFileSync("manager.bin"));
const registry = createFileRegistry(descriptorSet);
const independentlyLoaded = registry.getService("review.binding.ContractService");
assert.ok(independentlyLoaded);
defineContract({ service: independentlyLoaded, api: contract.api, version: contract.version, fingerprint: contract.fingerprint, graph: runtimeGraph });
const changedSet = fromBinary(FileDescriptorSetSchema, readFileSync("manager.bin"));
const dependency = changedSet.file.find((file) => file.name === "imported.proto");
assert.ok(dependency);
dependency.messageType[0].field[0].jsonName = "differentName";
const mismatched = createFileRegistry(changedSet).getService("review.binding.ContractService");
assert.ok(mismatched);
assert.throws(() => defineContract({ service: mismatched, api: contract.api, version: contract.version, fingerprint: contract.fingerprint, graph: runtimeGraph }), /runtime graph/);
console.log("PASS genuine protoc contract + generated ES binding; imported mismatch rejected");
