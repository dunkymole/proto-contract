import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

import { bindPackedBridgeToLock } from "./verify-bridge-lock.mjs";

function fixture(t) {
  const directory = mkdtempSync(join(tmpdir(), "bridge-lock-"));
  t.after(() => rmSync(directory, { recursive: true, force: true }));
  const lockPath = join(directory, "package-lock.json");
  const manifestPath = join(directory, "package.json");
  const tarballPath = join(directory, "bridge.tgz");
  const metadata = { dependencies: { "@bufbuild/protobuf": "2.15.0" }, engines: { node: ">=24" } };
  writeFileSync(lockPath, JSON.stringify({ packages: { "node_modules/@dunkymole/grpc-bridge": { version: "0.2.0", ...metadata } } }));
  writeFileSync(manifestPath, JSON.stringify({ name: "@dunkymole/grpc-bridge", version: "0.2.0", ...metadata }));
  writeFileSync(tarballPath, "fixture tarball");
  return { directory, lockPath, manifestPath, tarballPath };
}

test("binds the exact packed tarball hash when package metadata matches", (t) => {
  const files = fixture(t);
  const actual = bindPackedBridgeToLock(files);
  const expected = `sha512-${createHash("sha512").update(readFileSync(files.tarballPath)).digest("base64")}`;
  assert.equal(actual, expected);
  assert.equal(JSON.parse(readFileSync(files.lockPath, "utf8")).packages["node_modules/@dunkymole/grpc-bridge"].integrity, expected);
});

test("rejects a companion ref that adds a runtime dependency missing from the lock", (t) => {
  const files = fixture(t);
  const manifest = JSON.parse(readFileSync(files.manifestPath, "utf8"));
  manifest.dependencies["new-runtime-package"] = "1.0.0";
  writeFileSync(files.manifestPath, JSON.stringify(manifest));
  assert.throws(() => bindPackedBridgeToLock(files), /dependencies differs from the consumer lock/);
});

test("rejects a companion ref with a different package version", (t) => {
  const files = fixture(t);
  const manifest = JSON.parse(readFileSync(files.manifestPath, "utf8"));
  manifest.version = "0.3.0";
  writeFileSync(files.manifestPath, JSON.stringify(manifest));
  assert.throws(() => bindPackedBridgeToLock(files), /name\/version differs from the consumer lock/);
});
